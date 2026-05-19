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
	"github.com/twitter/shared/healthz"
	"github.com/twitter/shared/logger"
	sharedotel "github.com/twitter/shared/otel"
	"github.com/twitter/shared/outbox"
	db "github.com/twitter/user-service/db/sqlc"
	"github.com/twitter/user-service/internal/keycloak"
	"github.com/twitter/user-service/internal/server"
	"github.com/twitter/user-service/internal/user"
)

func main() {
	log := logger.New("user-service")

	shutdown := sharedotel.Setup("user-service")
	defer shutdown()

	dbURL := envutil.MustEnv("DATABASE_URL")
	dbmigrate.Run(dbURL, log)

	poolCfg, err := pgxpool.ParseConfig(dbURL)
	if err != nil {
		log.Error("parse database url", slog.Any("err", err))
		os.Exit(1)
	}
	poolCfg.MaxConns = 10
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		log.Error("connect to database", slog.Any("err", err))
		os.Exit(1)
	}
	defer pool.Close()

	rdb := redis.NewClient(&redis.Options{Addr: envutil.MustEnv("REDIS_URL")})
	defer rdb.Close()
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		log.Error("redis ping", slog.Any("err", err))
		os.Exit(1)
	}

	kw := &kafka.Writer{
		Addr:         kafka.TCP(envutil.MustEnv("KAFKA_BROKERS")),
		Balancer:     &kafka.Hash{},
		RequiredAcks: kafka.RequireOne,
	}
	defer kw.Close()

	kc := keycloak.New(
		envutil.MustEnv("KEYCLOAK_BASE_URL"),
		envutil.MustEnv("KEYCLOAK_REALM"),
		envutil.MustEnv("KEYCLOAK_ADMIN_CLIENT_ID"),
		envutil.MustEnv("KEYCLOAK_ADMIN_CLIENT_SECRET"),
		log,
	)

	serviceToken := envutil.MustEnv("SERVICE_TOKEN")
	userSvc := user.New(db.NewStore(pool), rdb, kw, kc, log)
	srv := server.New(userSvc, serviceToken, log,
		healthz.Func("postgres", pool.Ping),
		healthz.Func("redis", func(ctx context.Context) error { return rdb.Ping(ctx).Err() }),
	)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Sync user:counts:* from Postgres on boot — handles fresh Redis (dev volume
	// wipe, ephemeral AWS env) and any drift from a previous crash mid-write.
	if err := userSvc.RefreshAllCounts(ctx); err != nil {
		log.Warn("refresh user counts", slog.Any("err", err))
	}

	go outbox.Run(ctx, log, "user-service", userSvc.FlushOutbox)

	srv.Start(ctx,
		":"+envutil.GetEnv("PORT", "8080"),
		":"+envutil.GetEnv("GRPC_PORT", "9090"),
	)
}
