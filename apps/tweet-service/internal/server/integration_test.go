//go:build integration

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/segmentio/kafka-go"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/twitter/shared/dbmigrate"
	db "github.com/twitter/tweet-service/db/sqlc"
	"github.com/twitter/tweet-service/internal/server"
	"github.com/twitter/tweet-service/internal/tweet"
)

const (
	testUserID    = "usr_test01"
	testUserEmail = "test@example.com"
	testUsername  = "testuser"
)

type testEnv struct {
	srv  *httptest.Server
	pool *pgxpool.Pool
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

	migrateURL, err := pgc.ConnectionString(ctx, "sslmode=disable", "x-migrations-table=tweet_migrations")
	if err != nil {
		t.Fatalf("get migrate url: %v", err)
	}
	if err := dbmigrate.RunSource("file://../../migrations", migrateURL); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	poolURL, err := pgc.ConnectionString(ctx, "sslmode=disable", "search_path=tweet")
	if err != nil {
		t.Fatalf("get pool url: %v", err)
	}
	pool, err := pgxpool.New(ctx, poolURL)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	t.Cleanup(pool.Close)

	// nil writer: tweet-service guards against nil kb so no Kafka broker is required.
	var kw *kafka.Writer

	tweetSvc := tweet.New(db.NewStore(pool), kw, nil, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	srv := server.New(tweetSvc, "test-service-token", slog.New(slog.NewTextHandler(os.Stderr, nil)))

	return &testEnv{
		srv:  httptest.NewServer(srv.Handler()),
		pool: pool,
	}
}

func (e *testEnv) do(t *testing.T, method, path string, body any) *http.Response {
	t.Helper()
	var b []byte
	if body != nil {
		b, _ = json.Marshal(body)
	}
	req, _ := http.NewRequest(method, e.srv.URL+path, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", testUserID)
	req.Header.Set("X-User-Email", testUserEmail)
	req.Header.Set("X-User-Username", testUsername)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	return resp
}

func decode(t *testing.T, resp *http.Response, dst any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func TestIntegration_CreateAndGetTweet(t *testing.T) {
	e := newTestEnv(t)
	defer e.srv.Close()

	resp := e.do(t, http.MethodPost, "/v1/tweets", map[string]any{"body": "Hello #golang world!"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create tweet: expected 201, got %d", resp.StatusCode)
	}
	var created map[string]any
	decode(t, resp, &created)
	tweetID := created["id"].(string)

	resp = e.do(t, http.MethodGet, "/v1/tweets/"+tweetID, nil)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("get tweet: expected 200, got %d", resp.StatusCode)
	}
	var got map[string]any
	decode(t, resp, &got)
	if got["id"] != tweetID {
		t.Fatalf("tweet id mismatch: want %s got %v", tweetID, got["id"])
	}
	tags := got["hashtags"].([]any)
	if len(tags) == 0 || tags[0] != "#golang" {
		t.Fatalf("expected #golang hashtag, got %v", tags)
	}
}

func TestIntegration_OutboxRowCreated(t *testing.T) {
	e := newTestEnv(t)
	defer e.srv.Close()

	resp := e.do(t, http.MethodPost, "/v1/tweets", map[string]any{"body": "outbox test"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("create tweet: expected 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	var count int
	err := e.pool.QueryRow(context.Background(),
		"SELECT COUNT(*) FROM outbox WHERE sent_at IS NULL").Scan(&count)
	if err != nil {
		t.Fatalf("query outbox: %v", err)
	}
	if count == 0 {
		t.Fatal("expected at least one pending outbox row after tweet creation")
	}
}

func TestIntegration_Like_Idempotent(t *testing.T) {
	e := newTestEnv(t)
	defer e.srv.Close()

	var created map[string]any
	decode(t, e.do(t, http.MethodPost, "/v1/tweets", map[string]any{"body": "likeable"}), &created)
	tweetID := created["id"].(string)

	for i := 0; i < 2; i++ {
		resp := e.do(t, http.MethodPost, "/v1/tweets/"+tweetID+"/like", nil)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("like #%d: expected 200, got %d", i+1, resp.StatusCode)
		}
		resp.Body.Close()
	}

	var got map[string]any
	decode(t, e.do(t, http.MethodGet, "/v1/tweets/"+tweetID, nil), &got)
	if int(got["like_count"].(float64)) != 1 {
		t.Fatalf("expected like_count 1 (idempotent), got %v", got["like_count"])
	}
}

func TestIntegration_SelfRetweet_Allowed(t *testing.T) {
	e := newTestEnv(t)
	defer e.srv.Close()

	var created map[string]any
	decode(t, e.do(t, http.MethodPost, "/v1/tweets", map[string]any{"body": "my tweet"}), &created)
	tweetID := created["id"].(string)

	resp := e.do(t, http.MethodPost, "/v1/tweets/"+tweetID+"/retweet", nil)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("self-retweet: expected 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestIntegration_Reply_Thread(t *testing.T) {
	e := newTestEnv(t)
	defer e.srv.Close()

	var parent map[string]any
	decode(t, e.do(t, http.MethodPost, "/v1/tweets", map[string]any{"body": "parent tweet"}), &parent)
	parentID := parent["id"].(string)

	resp := e.do(t, http.MethodPost, "/v1/tweets/"+parentID+"/reply", map[string]any{"body": "a reply"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("reply: expected 201, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	var got map[string]any
	decode(t, e.do(t, http.MethodGet, "/v1/tweets/"+parentID, nil), &got)
	if int(got["reply_count"].(float64)) != 1 {
		t.Fatalf("expected reply_count 1, got %v", got["reply_count"])
	}

	var replyBody map[string]any
	decode(t, e.do(t, http.MethodGet, "/v1/tweets/"+parentID+"/replies", nil), &replyBody)
	if len(replyBody["tweets"].([]any)) != 1 {
		t.Fatalf("expected 1 reply, got %v", replyBody["tweets"])
	}
}

func TestIntegration_DeleteTweet(t *testing.T) {
	e := newTestEnv(t)
	defer e.srv.Close()

	var created map[string]any
	decode(t, e.do(t, http.MethodPost, "/v1/tweets", map[string]any{"body": "delete me"}), &created)
	tweetID := created["id"].(string)

	resp := e.do(t, http.MethodDelete, "/v1/tweets/"+tweetID, nil)
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("delete: expected 204, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	resp = e.do(t, http.MethodGet, "/v1/tweets/"+tweetID, nil)
	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("after delete: expected 404, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
