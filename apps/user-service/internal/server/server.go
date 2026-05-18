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
	"github.com/twitter/user-service/internal/user"
)

type Server struct {
	user         *user.Service
	serviceToken string
	log          *slog.Logger
	checkers     []healthz.Checker
}

func New(user *user.Service, serviceToken string, log *slog.Logger, checkers ...healthz.Checker) *Server {
	return &Server{user: user, serviceToken: serviceToken, log: log, checkers: checkers}
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
	r.Use(reqlog.TraceContext("user-service"))
	r.Use(reqlog.RequestID())
	r.Use(reqlog.StructuredLogger(s.log))
	r.Use(metrics.GinMiddleware("user-service"))

	r.GET("/healthz", healthz.Handler("user-service", s.checkers...))
	r.GET("/livez", healthz.Livez("user-service"))
	r.GET("/metrics", metrics.MetricsHandler())

	internal := r.Group("/internal", auth.ServiceTokenMiddleware(s.serviceToken))
	internal.POST("/provision", s.handleProvision)

	v1 := r.Group("/v1")

	authed := v1.Group("/", auth.HeadersMiddleware())
	authed.GET("/users/me", s.handleGetMe)
	authed.PATCH("/users/me", s.handleUpdateMe)
	// /users/suggestions must come before /users/:id so the literal segment wins.
	authed.GET("/users/suggestions", s.handleListSuggestions)
	authed.GET("/users/:id", s.handleGetUser)
	authed.POST("/users/:id/follow", s.handleFollow)
	authed.DELETE("/users/:id/follow", s.handleUnfollow)
	authed.GET("/users/:id/followers", s.handleListFollowers)
	authed.GET("/users/:id/following", s.handleListFollowing)

	return r
}
