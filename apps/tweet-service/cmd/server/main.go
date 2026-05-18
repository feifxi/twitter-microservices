package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"

	"github.com/twitter/shared/dbmigrate"
	"github.com/twitter/shared/envutil"
	"github.com/twitter/shared/events"
	"github.com/twitter/shared/healthz"
	"github.com/twitter/shared/logger"
	sharedotel "github.com/twitter/shared/otel"
	"github.com/twitter/shared/outbox"
	db "github.com/twitter/tweet-service/db/sqlc"
	"github.com/twitter/tweet-service/internal/server"
	"github.com/twitter/tweet-service/internal/tweet"
)

func main() {
	log := logger.New("tweet-service")

	shutdown := sharedotel.Setup("tweet-service")
	defer shutdown()

	dbURL := envutil.MustEnv("DATABASE_URL")
	dbmigrate.Run(dbURL, log)

	cfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		log.Error("parse db config", slog.Any("err", err))
		os.Exit(1)
	}
	cfg.MaxConns = 10
	pool, err := pgxpool.NewWithConfig(context.Background(), cfg)
	if err != nil {
		log.Error("connect db", slog.Any("err", err))
		os.Exit(1)
	}
	defer pool.Close()

	kw := &kafka.Writer{
		Addr:         kafka.TCP(envutil.MustEnv("KAFKA_BROKERS")),
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireOne,
	}
	defer kw.Close()

	rdb := redis.NewClient(&redis.Options{Addr: envutil.MustEnv("REDIS_URL")})
	defer rdb.Close()

	serviceToken := envutil.MustEnv("SERVICE_TOKEN")

	tweetSvc := tweet.New(db.NewStore(pool), kw, rdb, log)

	userReader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{envutil.MustEnv("KAFKA_BROKERS")},
		GroupID: "tweet-service-user-cg",
		GroupTopics: []string{
			events.TopicUserCreated,
			events.TopicUserUpdated,
		},
		MinBytes: 1,
		MaxBytes: 1e6,
		MaxWait:  500 * time.Millisecond,
	})
	defer userReader.Close()

	srv := server.New(tweetSvc, serviceToken, log,
		healthz.Func("postgres", pool.Ping),
		healthz.Func("redis", func(ctx context.Context) error { return rdb.Ping(ctx).Err() }),
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go tweetSvc.RunUserSnapshotConsumer(ctx, userReader)
	go outbox.Run(ctx, log, "tweet-service", tweetSvc.FlushOutbox)

	srv.Start(ctx,
		":"+envutil.GetEnv("PORT", "8080"),
		":"+envutil.GetEnv("GRPC_PORT", "9090"),
	)
}
