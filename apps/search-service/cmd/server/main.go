package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"github.com/twitter/shared/envutil"
	"github.com/twitter/shared/events"
	"github.com/twitter/shared/healthz"
	"github.com/twitter/shared/logger"
	sharedotel "github.com/twitter/shared/otel"
	"github.com/twitter/search-service/internal/embeddings"
	"github.com/twitter/search-service/internal/enrich"
	"github.com/twitter/search-service/internal/indexer"
	"github.com/twitter/search-service/internal/opensearch"
	"github.com/twitter/search-service/internal/search"
	"github.com/twitter/search-service/internal/server"
	"github.com/twitter/search-service/internal/tweetclient"
	"github.com/twitter/search-service/internal/userclient"
)

func main() {
	log := logger.New("search-service")

	shutdown := sharedotel.Setup("search-service")
	defer shutdown()

	osClient, err := opensearch.New(envutil.MustEnv("OPENSEARCH_URL"))
	if err != nil {
		log.Error("init opensearch client", slog.Any("err", err))
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	initCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	if err := osClient.EnsureIndices(initCtx); err != nil {
		log.Error("ensure opensearch indices", slog.Any("err", err))
		cancel()
		os.Exit(1)
	}
	cancel()

	embedClient := embeddings.New(
		envutil.MustEnv("OPENAI_API_KEY"),
		envutil.GetEnv("OPENAI_EMBEDDING_MODEL", ""),
		0,
	)

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{envutil.MustEnv("KAFKA_BROKERS")},
		GroupID: "search-service-cg",
		GroupTopics: []string{
			events.TopicTweetCreated,
			events.TopicTweetDeleted,
			events.TopicUserCreated,
			events.TopicUserUpdated,
		},
		MinBytes: 1,
		MaxBytes: 1e6,
		MaxWait:  500 * time.Millisecond,
	})
	defer reader.Close()

	dlqWriter := &kafka.Writer{
		Addr:         kafka.TCP(envutil.MustEnv("KAFKA_BROKERS")),
		Topic:        events.TopicSearchDLQ,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireOne,
	}
	defer dlqWriter.Close()

	rdb := redis.NewClient(&redis.Options{Addr: envutil.MustEnv("REDIS_URL")})
	defer rdb.Close()

	userSvc, err := userclient.New(
		envutil.MustEnv("USER_SERVICE_GRPC_ADDR"),
		envutil.MustEnv("SERVICE_TOKEN"),
		rdb,
	)
	if err != nil {
		log.Error("init user-service client", slog.Any("err", err))
		os.Exit(1)
	}
	defer userSvc.Close()

	tweetSvc, err := tweetclient.New(
		envutil.MustEnv("TWEET_SERVICE_GRPC_ADDR"),
		envutil.MustEnv("SERVICE_TOKEN"),
	)
	if err != nil {
		log.Error("init tweet-service client", slog.Any("err", err))
		os.Exit(1)
	}
	defer tweetSvc.Close()

	tweetEnricher := enrich.New(rdb, tweetSvc)

	idx := indexer.New(osClient, embedClient, log).WithDLQ(dlqWriter)
	searchSvc := search.New(osClient, embedClient, userSvc, tweetEnricher, log)
	srv := server.New(searchSvc, log,
		healthz.Func("redis", func(ctx context.Context) error { return rdb.Ping(ctx).Err() }),
		healthz.Func("opensearch", osClient.Ping),
	)

	go idx.Run(ctx, reader)

	srv.Start(ctx, ":"+envutil.GetEnv("PORT", "8080"))
}
