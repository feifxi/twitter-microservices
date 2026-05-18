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
	"github.com/twitter/tweet-service/internal/tweet"
)

type Server struct {
	tweet        *tweet.Service
	serviceToken string
	log          *slog.Logger
	checkers     []healthz.Checker
}

func New(tweet *tweet.Service, serviceToken string, log *slog.Logger, checkers ...healthz.Checker) *Server {
	return &Server{tweet: tweet, serviceToken: serviceToken, log: log, checkers: checkers}
}

func (s *Server) Handler() http.Handler {
	return s.routes()
}

func (s *Server) Start(ctx context.Context, httpAddr, grpcAddr string) {
	grpcSrv := s.startGRPC(grpcAddr)

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
	grpcSrv.GracefulStop()
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
	r.Use(reqlog.TraceContext("tweet-service"))
	r.Use(reqlog.RequestID())
	r.Use(reqlog.StructuredLogger(s.log))
	r.Use(metrics.GinMiddleware("tweet-service"))

	r.GET("/healthz", healthz.Handler("tweet-service", s.checkers...))
	r.GET("/livez", healthz.Livez("tweet-service"))
	r.GET("/metrics", metrics.MetricsHandler())

	protected := r.Group("/v1", auth.HeadersMiddleware())
	protected.POST("/tweets", s.handleCreateTweet)
	protected.GET("/tweets/:id", s.handleGetTweet)
	protected.DELETE("/tweets/:id", s.handleDeleteTweet)
	protected.POST("/tweets/:id/like", s.handleLike)
	protected.DELETE("/tweets/:id/like", s.handleUnlike)
	protected.POST("/tweets/:id/retweet", s.handleRetweet)
	protected.DELETE("/tweets/:id/retweet", s.handleUnretweet)
	protected.POST("/tweets/:id/reply", s.handleReply)
	protected.GET("/tweets/:id/replies", s.handleGetReplies)
	protected.GET("/users/:id/tweets", s.handleGetProfileTimeline)
	protected.GET("/users/:id/likes", s.handleGetUserLikes)

	return r
}
