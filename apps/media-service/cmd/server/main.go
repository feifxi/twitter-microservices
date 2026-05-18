package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/twitter/media-service/internal/media"
	"github.com/twitter/media-service/internal/server"
	"github.com/twitter/shared/envutil"
	"github.com/twitter/shared/logger"
	sharedotel "github.com/twitter/shared/otel"
)

func main() {
	log := logger.New("media-service")

	shutdown := sharedotel.Setup("media-service")
	defer shutdown()

	awsCfg, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(envutil.GetEnv("AWS_REGION", "ap-southeast-1")),
	)
	if err != nil {
		log.Error("load aws config", slog.Any("err", err))
		os.Exit(1)
	}

	// Presign endpoint must be reachable by the browser (the user's host),
	// not by the server (the Docker network). In dev these differ:
	//   server-side  → http://localstack:4566
	//   browser-side → http://localhost:4566
	// In prod (real S3) both env vars are unset and the SDK uses the regional
	// endpoint, which is reachable from both sides.
	presignEndpoint := envutil.GetEnv("AWS_S3_PRESIGN_ENDPOINT_URL", os.Getenv("AWS_ENDPOINT_URL"))
	s3PresignClient := s3.NewFromConfig(awsCfg, func(o *s3.Options) {
		if presignEndpoint != "" {
			o.BaseEndpoint = &presignEndpoint
			o.UsePathStyle = true
		}
	})

	mediaSvc := media.New(
		s3.NewPresignClient(s3PresignClient),
		envutil.MustEnv("S3_BUCKET"),
		envutil.MustEnv("MEDIA_PUBLIC_URL_BASE"),
	)

	srv := server.New(mediaSvc, log)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	srv.Start(ctx, ":"+envutil.GetEnv("PORT", "8080"))
}
