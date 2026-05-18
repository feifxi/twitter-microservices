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
	"github.com/twitter/feed-service/internal/consumer"
	"github.com/twitter/feed-service/internal/server"
	"github.com/twitter/feed-service/internal/feed"
	"github.com/twitter/feed-service/internal/tweetclient"
	"github.com/twitter/feed-service/internal/userclient"
)

func main() {
	log := logger.New("feed-service")

	shutdown := sharedotel.Setup("feed-service")
	defer shutdown()

	rdb := redis.NewClient(&redis.Options{Addr: envutil.MustEnv("REDIS_URL")})
	defer rdb.Close()
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Error("redis ping", slog.Any("err", err))
		os.Exit(1)
	}

	serviceToken := envutil.MustEnv("SERVICE_TOKEN")

	userSvc, err := userclient.New(envutil.MustEnv("USER_SERVICE_GRPC_ADDR"), serviceToken)
	if err != nil {
		log.Error("init user grpc client", slog.Any("err", err))
		os.Exit(1)
	}
	defer userSvc.Close()

	tweetSvc, err := tweetclient.New(envutil.MustEnv("TWEET_SERVICE_GRPC_ADDR"), serviceToken)
	if err != nil {
		log.Error("init tweet grpc client", slog.Any("err", err))
		os.Exit(1)
	}
	defer tweetSvc.Close()

	dlqWriter := &kafka.Writer{
		Addr:         kafka.TCP(envutil.MustEnv("KAFKA_BROKERS")),
		Topic:        events.TopicFeedDLQ,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireOne,
	}
	defer dlqWriter.Close()

	c := consumer.New(rdb, userSvc, log).WithDLQ(dlqWriter)

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{envutil.MustEnv("KAFKA_BROKERS")},
		GroupID: "feed-service-cg",
		GroupTopics: []string{
			events.TopicTweetCreated,
			events.TopicTweetRetweeted,
			events.TopicTweetUnretweeted,
			events.TopicTweetDeleted,
			events.TopicTweetLiked,
			events.TopicUserFollowed,
			events.TopicUserUpdated,
		},
		MinBytes: 1,
		MaxBytes: 1e6,
		MaxWait:  500 * time.Millisecond,
	})
	defer reader.Close()

	feedSvc := feed.New(c, tweetSvc, userSvc, log)
	srv := server.New(feedSvc, log,
		healthz.Func("redis", func(ctx context.Context) error { return rdb.Ping(ctx).Err() }),
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go c.RunTweetConsumer(ctx, reader)

	srv.Start(ctx, ":"+envutil.GetEnv("PORT", "8080"))
}
