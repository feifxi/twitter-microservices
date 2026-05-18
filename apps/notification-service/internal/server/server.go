package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
	"github.com/twitter/shared/auth"
	"github.com/twitter/shared/healthz"
	"github.com/twitter/shared/metrics"
	"github.com/twitter/shared/reqlog"
	"github.com/twitter/notification-service/internal/notification"
)

const (
	userSnapshotKey  = "user:snapshot:%s"
	tweetSnapshotKey = "tweet:snapshot:%s"
	previewMaxRunes  = 100
)

type Server struct {
	notif    *notification.Service
	hub      *notification.Hub
	rdb      *redis.Client
	log      *slog.Logger
	checkers []healthz.Checker
}

func New(notif *notification.Service, hub *notification.Hub, rdb *redis.Client, log *slog.Logger, checkers ...healthz.Checker) *Server {
	return &Server{notif: notif, hub: hub, rdb: rdb, log: log, checkers: checkers}
}

// lookupActors fetches actor profiles for the given IDs from Redis snapshots via a pipeline.
func (s *Server) lookupActors(ctx context.Context, ids []string) map[string]actorInfo {
	unique := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id != "" {
			unique[id] = struct{}{}
		}
	}
	type entry struct {
		id  string
		cmd *redis.MapStringStringCmd
	}
	cmds := make([]entry, 0, len(unique))
	pipe := s.rdb.Pipeline()
	for id := range unique {
		cmds = append(cmds, entry{id: id, cmd: pipe.HGetAll(ctx, fmt.Sprintf(userSnapshotKey, id))})
	}
	pipe.Exec(ctx) //nolint:errcheck — individual cmd results checked below
	result := make(map[string]actorInfo, len(unique))
	for _, e := range cmds {
		fields, err := e.cmd.Result()
		if err != nil || len(fields) == 0 {
			result[e.id] = actorInfo{ID: e.id}
			continue
		}
		result[e.id] = actorInfo{
			ID:          e.id,
			Username:    fields["username"],
			DisplayName: fields["display_name"],
			AvatarURL:   fields["avatar_url"],
		}
	}
	return result
}

func (s *Server) lookupActor(ctx context.Context, id string) actorInfo {
	actors := s.lookupActors(ctx, []string{id})
	return actors[id]
}

// lookupTweetPreviews fetches tweet body snippets from tweet:snapshot hashes via a pipeline.
// Only IDs that have a non-empty body produce an entry; nil means no preview available.
func (s *Server) lookupTweetPreviews(ctx context.Context, tweetIDs []string) map[string]*string {
	unique := make(map[string]struct{}, len(tweetIDs))
	for _, id := range tweetIDs {
		if id != "" {
			unique[id] = struct{}{}
		}
	}
	type entry struct {
		id  string
		cmd *redis.StringCmd
	}
	cmds := make([]entry, 0, len(unique))
	pipe := s.rdb.Pipeline()
	for id := range unique {
		cmds = append(cmds, entry{id: id, cmd: pipe.HGet(ctx, fmt.Sprintf(tweetSnapshotKey, id), "body")})
	}
	pipe.Exec(ctx) //nolint:errcheck — individual cmd results checked below
	result := make(map[string]*string, len(unique))
	for _, e := range cmds {
		body, err := e.cmd.Result()
		if err != nil || body == "" {
			continue
		}
		preview := truncate(body, previewMaxRunes)
		result[e.id] = &preview
	}
	return result
}

func (s *Server) lookupTweetPreview(ctx context.Context, tweetID string) *string {
	if tweetID == "" {
		return nil
	}
	previews := s.lookupTweetPreviews(ctx, []string{tweetID})
	return previews[tweetID]
}

// truncate cuts s to at most n runes, appending "…" if truncated.
func truncate(s string, n int) string {
	runes := []rune(s)
	if len(runes) <= n {
		return s
	}
	return string(runes[:n]) + "…"
}

func (s *Server) Handler() http.Handler {
	return s.routes()
}

func (s *Server) Start(ctx context.Context, addr string) {
	httpServer := &http.Server{
		Addr:         addr,
		Handler:      s.routes(),
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 0, // SSE connections are long-lived
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
	r.Use(reqlog.TraceContext("notification-service"))
	r.Use(reqlog.RequestID())
	r.Use(reqlog.StructuredLogger(s.log))
	r.Use(metrics.GinMiddleware("notification-service"))

	r.GET("/healthz", healthz.Handler("notification-service", s.checkers...))
	r.GET("/livez", healthz.Livez("notification-service"))
	r.GET("/metrics", metrics.MetricsHandler())

	protected := r.Group("/v1", auth.HeadersMiddleware())
	protected.GET("/notifications", s.handleGetNotifications)
	protected.PATCH("/notifications/read", s.handleMarkAllRead)
	protected.PATCH("/notifications/:id/read", s.handleMarkRead)
	protected.GET("/notifications/stream", s.handleStream)

	return r
}
