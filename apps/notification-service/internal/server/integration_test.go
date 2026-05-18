//go:build integration

package server_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/alicebob/miniredis/v2"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	db "github.com/twitter/notification-service/db/sqlc"
	"github.com/twitter/notification-service/internal/notification"
	"github.com/twitter/notification-service/internal/server"
	"github.com/twitter/shared/dbmigrate"
)

// notifResp mirrors server.notificationResponse for test decoding.
type notifResp struct {
	ID               string  `json:"id"`
	ActorID          string  `json:"actor_id"`
	ActorUsername    string  `json:"actor_username"`
	ActorDisplayName string  `json:"actor_display_name"`
	ActorAvatarURL   *string `json:"actor_avatar_url"`
	Type             string  `json:"type"`
	TweetID          *string `json:"tweet_id"`
	TweetPreview     *string `json:"tweet_preview"`
	ReadAt           *string `json:"read_at"`
	CreatedAt        string  `json:"created_at"`
}

const (
	testUserID    = "usr_test01"
	testActorID   = "usr_test02"
	testTweetID   = "tw_test01"
	testUserEmail = "test@example.com"
	testUsername  = "testuser"
)

type testEnv struct {
	srv      *httptest.Server
	notifSvc *notification.Service
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	ctx := context.Background()

	pgc, err := tcpostgres.Run(ctx,
		"postgres:16-alpine",
		tcpostgres.WithDatabase("testdb"),
		tcpostgres.WithUsername("test"),
		tcpostgres.WithPassword("test"),
		tcpostgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { pgc.Terminate(ctx) })

	migrateURL, err := pgc.ConnectionString(ctx, "sslmode=disable", "x-migrations-table=notification_migrations")
	if err != nil {
		t.Fatalf("get migrate url: %v", err)
	}
	if err := dbmigrate.RunSource("file://../../migrations", migrateURL); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	poolURL, err := pgc.ConnectionString(ctx, "sslmode=disable", "search_path=notification")
	if err != nil {
		t.Fatalf("get pool url: %v", err)
	}
	pool, err := pgxpool.New(ctx, poolURL)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	t.Cleanup(pool.Close)

	os.Setenv("SERVICE_TOKEN", "test-service-token")

	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := notification.NewHub(nil, log) // nil Redis: hub runs in-process only (no pub/sub) for tests
	notifSvc := notification.New(db.NewStore(pool), hub, log)
	srv := server.New(notifSvc, hub, rdb, log)

	return &testEnv{
		srv:      httptest.NewServer(srv.Handler()),
		notifSvc: notifSvc,
	}
}

func authHeader(userID, email, username string) http.Header {
	h := http.Header{}
	h.Set("X-User-ID", userID)
	h.Set("X-User-Email", email)
	h.Set("X-User-Username", username)
	return h
}

func doReq(t *testing.T, srv *httptest.Server, method, path string, headers http.Header) *http.Response {
	t.Helper()
	req, err := http.NewRequest(method, srv.URL+path, nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	for k, vs := range headers {
		for _, v := range vs {
			req.Header.Set(k, v)
		}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

func TestGetNotifications_Empty(t *testing.T) {
	env := newTestEnv(t)
	defer env.srv.Close()

	resp := doReq(t, env.srv, http.MethodGet, "/v1/notifications",
		authHeader(testUserID, testUserEmail, testUsername))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Notifications []notifResp `json:"notifications"`
		NextCursor    string      `json:"next_cursor"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Notifications) != 0 {
		t.Errorf("expected 0 notifications, got %d", len(body.Notifications))
	}
}

func TestGetNotifications_AfterCreate(t *testing.T) {
	env := newTestEnv(t)
	defer env.srv.Close()

	// Create a like notification: actor likes testUserID's tweet
	if err := env.notifSvc.CreateFromLike(context.Background(), 0, testActorID, testUserID, testTweetID); err != nil {
		t.Fatalf("create like: %v", err)
	}

	resp := doReq(t, env.srv, http.MethodGet, "/v1/notifications",
		authHeader(testUserID, testUserEmail, testUsername))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Notifications []notifResp `json:"notifications"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Notifications) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(body.Notifications))
	}
	n := body.Notifications[0]
	if n.ActorID != testActorID {
		t.Errorf("actor_id: want %s, got %s", testActorID, n.ActorID)
	}
	if n.Type != "like" {
		t.Errorf("type: want like, got %s", n.Type)
	}
}

func TestMarkRead(t *testing.T) {
	env := newTestEnv(t)
	defer env.srv.Close()

	if err := env.notifSvc.CreateFromLike(context.Background(), 0, testActorID, testUserID, testTweetID); err != nil {
		t.Fatalf("create like: %v", err)
	}

	// Get the notification ID
	resp := doReq(t, env.srv, http.MethodGet, "/v1/notifications",
		authHeader(testUserID, testUserEmail, testUsername))
	var listBody struct {
		Notifications []notifResp `json:"notifications"`
	}
	json.NewDecoder(resp.Body).Decode(&listBody)
	resp.Body.Close()

	notifID := listBody.Notifications[0].ID

	// Mark it read
	resp2 := doReq(t, env.srv, http.MethodPatch, "/v1/notifications/"+notifID+"/read",
		authHeader(testUserID, testUserEmail, testUsername))
	defer resp2.Body.Close()

	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp2.StatusCode)
	}

	var n notifResp
	if err := json.NewDecoder(resp2.Body).Decode(&n); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if n.ReadAt == nil {
		t.Error("expected read_at to be set")
	}
}

func TestMarkRead_NotFound(t *testing.T) {
	env := newTestEnv(t)
	defer env.srv.Close()

	resp := doReq(t, env.srv, http.MethodPatch, "/v1/notifications/ntf_doesnotexist/read",
		authHeader(testUserID, testUserEmail, testUsername))
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", resp.StatusCode)
	}
}

func TestMarkAllRead(t *testing.T) {
	env := newTestEnv(t)
	defer env.srv.Close()

	// Create two notifications for testUserID
	if err := env.notifSvc.CreateFromLike(context.Background(), 0, testActorID, testUserID, testTweetID); err != nil {
		t.Fatalf("create like: %v", err)
	}
	if err := env.notifSvc.CreateFromFollow(context.Background(), 0, testActorID, testUserID); err != nil {
		t.Fatalf("create follow: %v", err)
	}

	// Mark all read
	resp := doReq(t, env.srv, http.MethodPatch, "/v1/notifications/read",
		authHeader(testUserID, testUserEmail, testUsername))
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	// Verify: marking already-read notification returns 404 (read_at IS NULL filter)
	listResp := doReq(t, env.srv, http.MethodGet, "/v1/notifications",
		authHeader(testUserID, testUserEmail, testUsername))
	var body struct {
		Notifications []notifResp `json:"notifications"`
	}
	json.NewDecoder(listResp.Body).Decode(&body)
	listResp.Body.Close()

	for _, n := range body.Notifications {
		if n.ReadAt == nil {
			t.Errorf("notification %s should be read after mark-all", n.ID)
		}
	}
}

func TestSelfNotification_Skipped(t *testing.T) {
	env := newTestEnv(t)
	defer env.srv.Close()

	// Actor and author are the same — should be a no-op
	if err := env.notifSvc.CreateFromLike(context.Background(), 0, testUserID, testUserID, testTweetID); err != nil {
		t.Fatalf("create like: %v", err)
	}

	resp := doReq(t, env.srv, http.MethodGet, "/v1/notifications",
		authHeader(testUserID, testUserEmail, testUsername))
	defer resp.Body.Close()

	var body struct {
		Notifications []notifResp `json:"notifications"`
	}
	json.NewDecoder(resp.Body).Decode(&body)

	if len(body.Notifications) != 0 {
		t.Errorf("self-like should produce 0 notifications, got %d", len(body.Notifications))
	}
}
