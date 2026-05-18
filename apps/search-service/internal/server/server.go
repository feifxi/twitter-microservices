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
	"github.com/twitter/shared/metrics"
	"github.com/twitter/shared/reqlog"
	"github.com/twitter/search-service/internal/search"
)

type Server struct {
	search *search.Service
	log    *slog.Logger
}

func New(svc *search.Service, log *slog.Logger) *Server {
	return &Server{search: svc, log: log}
}

func (s *Server) Handler() http.Handler {
	return s.routes()
}

func (s *Server) Start(ctx context.Context, addr string) {
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      s.routes(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	go func() {
		s.log.Info("starting http server", slog.String("addr", addr))
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
	r.Use(reqlog.TraceContext("search-service"))
	r.Use(reqlog.RequestID())
	r.Use(reqlog.StructuredLogger(s.log))
	r.Use(metrics.GinMiddleware("search-service"))

	r.GET("/healthz", s.handleHealthz)
	r.GET("/metrics", metrics.MetricsHandler())

	protected := r.Group("/v1", auth.HeadersMiddleware())
	protected.GET("/search/tweets", s.handleSearchTweets)
	protected.GET("/search/users", s.handleSearchUsers)

	return r
}

func (s *Server) handleHealthz(c *gin.Context) {
	c.JSON(http.StatusOK, healthzResponse{Status: "ok", Service: "search-service"})
}
