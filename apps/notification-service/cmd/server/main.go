package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"github.com/twitter/shared/dbmigrate"
	"github.com/twitter/shared/envutil"
	"github.com/twitter/shared/events"
	"github.com/twitter/shared/logger"
	sharedotel "github.com/twitter/shared/otel"
	db "github.com/twitter/notification-service/db/sqlc"
	"github.com/twitter/notification-service/internal/consumer"
	"github.com/twitter/notification-service/internal/notification"
	"github.com/twitter/notification-service/internal/server"
)

func main() {
	log := logger.New("notification-service")

	shutdown := sharedotel.Setup("notification-service")
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

	rdb := redis.NewClient(&redis.Options{Addr: envutil.MustEnv("REDIS_URL")})
	defer rdb.Close()

	store := db.NewStore(pool)
	hub := notification.NewHub(rdb, log)
	notifSvc := notification.New(store, hub, log)

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers: []string{envutil.MustEnv("KAFKA_BROKERS")},
		GroupID: "notification-service-cg",
		GroupTopics: []string{
			events.TopicTweetLiked,
			events.TopicTweetRetweeted,
			events.TopicTweetUnretweeted,
			events.TopicTweetReplied,
			events.TopicUserFollowed,
		},
	})
	defer r.Close()

	dlqWriter := &kafka.Writer{
		Addr:         kafka.TCP(envutil.MustEnv("KAFKA_BROKERS")),
		Topic:        events.TopicNotificationDLQ,
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireOne,
	}
	defer dlqWriter.Close()

	cons := consumer.New(notifSvc, store, rdb, dlqWriter, log)
	srv := server.New(notifSvc, hub, rdb, log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	go hub.RunPubSub(ctx)
	go cons.Run(ctx, r)
	go notifSvc.RunDedupPrune(ctx)

	srv.Start(ctx, ":"+envutil.GetEnv("PORT", "8080"))
}
