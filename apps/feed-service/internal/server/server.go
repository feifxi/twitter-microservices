package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/twitter/shared/auth"
	"github.com/twitter/shared/healthz"
	"github.com/twitter/shared/metrics"
	"github.com/twitter/shared/reqlog"
	"github.com/twitter/feed-service/internal/feed"
)

type Server struct {
	feed     *feed.Service
	log      *slog.Logger
	checkers []healthz.Checker
}

func New(svc *feed.Service, log *slog.Logger, checkers ...healthz.Checker) *Server {
	return &Server{feed: svc, log: log, checkers: checkers}
}

func (s *Server) Handler() http.Handler {
	return s.routes()
}

func (s *Server) Start(ctx context.Context, httpAddr string) {
	httpServer := &http.Server{
		Addr:         httpAddr,
		Handler:      s.routes(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		s.log.Info("starting http server", slog.String("addr", httpAddr))
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			s.log.Error("http server error", slog.Any("err", err))
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	s.log.Info("shutting down")
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(shutCtx)
}

func (s *Server) routes() *gin.Engine {
	if os.Getenv("GIN_MODE") == "release" {
		gin.SetMode(gin.ReleaseMode)
	}
	r := gin.New()
	r.Use(gin.Recovery())
	r.Use(reqlog.TraceContext("feed-service"))
	r.Use(reqlog.RequestID())
	r.Use(reqlog.StructuredLogger(s.log))
	r.Use(metrics.GinMiddleware("feed-service"))

	r.GET("/healthz", healthz.Handler("feed-service", s.checkers...))
	r.GET("/livez", healthz.Livez("feed-service"))
	r.GET("/metrics", metrics.MetricsHandler())

	v1 := r.Group("/v1")

	feed := v1.Group("/feed")
	feed.GET("/trending", s.handleTrending)

	protected := feed.Group("/", auth.HeadersMiddleware())
	protected.GET("/following", s.handleFollowingFeed)
	protected.GET("/recommended", s.handleRecommendedFeed)

	return r
}
