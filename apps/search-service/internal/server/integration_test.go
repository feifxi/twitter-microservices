//go:build integration

package server_test

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"
	"github.com/twitter/search-service/internal/opensearch"
	"github.com/twitter/search-service/internal/search"
	"github.com/twitter/search-service/internal/server"
)

type stubEmbed struct{ vec []float32 }

func (s *stubEmbed) Embed(_ context.Context, _ string) ([]float32, error) { return s.vec, nil }

type stubUsers struct{}

func (stubUsers) BatchGetFollowerCounts(_ context.Context, _ []string) (map[string]int64, error) {
	return nil, nil
}
func (stubUsers) GetFollowState(_ context.Context, _ string, _ []string) (map[string]bool, error) {
	return nil, nil
}

type stubTweets struct{}

func (stubTweets) BatchAuthorSnapshots(_ context.Context, _ []string) map[string]search.AuthorSnapshot {
	return nil
}
func (stubTweets) BatchTweetCounts(_ context.Context, _ []string) map[string]search.TweetCounts {
	return nil
}
func (stubTweets) GetInteractions(_ context.Context, _ string, _ []string) (map[string]search.TweetInteraction, error) {
	return nil, nil
}

func startOpenSearch(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	c, err := testcontainers.GenericContainer(ctx, testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        "opensearchproject/opensearch:3.5.0",
			ExposedPorts: []string{"9200/tcp"},
			Env: map[string]string{
				"discovery.type":               "single-node",
				"OPENSEARCH_SECURITY_DISABLED":  "true",
				"OPENSEARCH_JAVA_OPTS":          "-Xms512m -Xmx512m",
			},
			WaitingFor: wait.ForHTTP("/_cluster/health").
				WithPort("9200/tcp").
				WithStartupTimeout(120 * time.Second),
		},
		Started: true,
	})
	if err != nil {
		t.Fatalf("start opensearch container: %v", err)
	}
	t.Cleanup(func() { c.Terminate(ctx) })

	host, err := c.Host(ctx)
	if err != nil {
		t.Fatalf("get container host: %v", err)
	}
	port, err := c.MappedPort(ctx, "9200")
	if err != nil {
		t.Fatalf("get container port: %v", err)
	}

	return fmt.Sprintf("http://%s:%s", host, port.Port())
}

func fixedVec() []float32 {
	v := make([]float32, 256)
	for i := range v {
		v[i] = 0.1
	}
	return v
}

type testEnv struct {
	srv      *httptest.Server
	osClient *opensearch.Client
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()

	addr := startOpenSearch(t)

	osClient, err := opensearch.New(addr)
	if err != nil {
		t.Fatalf("opensearch client: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if err := osClient.EnsureIndices(ctx); err != nil {
		t.Fatalf("ensure indices: %v", err)
	}

	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	svc := search.New(osClient, &stubEmbed{vec: fixedVec()}, &stubUsers{}, &stubTweets{}, log)
	srv := server.New(svc, log)

	return &testEnv{
		srv:      httptest.NewServer(srv.Handler()),
		osClient: osClient,
	}
}

func authHeader() http.Header {
	h := http.Header{}
	h.Set("X-User-ID", "usr_test01")
	h.Set("X-User-Email", "test@example.com")
	h.Set("X-User-Username", "testuser")
	return h
}

func get(t *testing.T, srv *httptest.Server, path string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodGet, srv.URL+path, nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	for k, vs := range authHeader() {
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

// without _refresh the default 1s interval makes tests flaky
func refreshIndices(t *testing.T, addr string) {
	t.Helper()
	resp, err := http.Post(addr+"/_refresh", "application/json", nil)
	if err != nil {
		t.Fatalf("refresh indices: %v", err)
	}
	resp.Body.Close()
}

func TestSearch_KeywordFindsTweet(t *testing.T) {
	env := newTestEnv(t)
	defer env.srv.Close()

	ctx := context.Background()
	if err := env.osClient.IndexTweet(ctx, opensearch.TweetDoc{
		ID:        "tw_001",
		AuthorID:  "usr_001",
		Body:      "golang rocks",
		Hashtags:  []string{"#golang"},
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("index tweet: %v", err)
	}
	refreshIndices(t, env.osClient.Addr())

	resp := get(t, env.srv, "/v1/search/tweets?q=golang&mode=keyword")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Tweets []struct {
			ID   string `json:"id"`
			Body string `json:"body"`
		} `json:"tweets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Tweets) == 0 {
		t.Fatal("expected at least 1 tweet, got 0")
	}
	if body.Tweets[0].ID != "tw_001" {
		t.Errorf("expected tw_001, got %s", body.Tweets[0].ID)
	}
}

func TestSearch_KeywordFindsByHashtag(t *testing.T) {
	env := newTestEnv(t)
	defer env.srv.Close()

	ctx := context.Background()
	if err := env.osClient.IndexTweet(ctx, opensearch.TweetDoc{
		ID:        "tw_002",
		AuthorID:  "usr_001",
		Body:      "hello world",
		Hashtags:  []string{"#golang"},
		CreatedAt: time.Now(),
	}); err != nil {
		t.Fatalf("index tweet: %v", err)
	}
	refreshIndices(t, env.osClient.Addr())

	resp := get(t, env.srv, "/v1/search/tweets?q=%23golang&mode=keyword")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Tweets []struct{ ID string `json:"id"` } `json:"tweets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Tweets) == 0 {
		t.Fatal("expected hashtag match, got 0 results")
	}
}

func TestSearch_SemanticFindsTweet(t *testing.T) {
	env := newTestEnv(t)
	defer env.srv.Close()

	ctx := context.Background()
	if err := env.osClient.IndexTweet(ctx, opensearch.TweetDoc{
		ID:        "tw_003",
		AuthorID:  "usr_001",
		Body:      "programming language",
		Hashtags:  []string{},
		CreatedAt: time.Now(),
		Vector:    fixedVec(),
	}); err != nil {
		t.Fatalf("index tweet: %v", err)
	}
	refreshIndices(t, env.osClient.Addr())

	resp := get(t, env.srv, "/v1/search/tweets?q=programming+language&mode=semantic")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Tweets []struct{ ID string `json:"id"` } `json:"tweets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Tweets) == 0 {
		t.Fatal("expected semantic match, got 0 results")
	}
	if body.Tweets[0].ID != "tw_003" {
		t.Errorf("expected tw_003, got %s", body.Tweets[0].ID)
	}
}

func TestSearch_UserSearch(t *testing.T) {
	env := newTestEnv(t)
	defer env.srv.Close()

	ctx := context.Background()
	if err := env.osClient.IndexUser(ctx, opensearch.UserDoc{
		ID:          "usr_001",
		Username:    "alice",
		DisplayName: "Alice Smith",
		Bio:         "Go developer",
		UpdatedAt:   time.Now(),
	}); err != nil {
		t.Fatalf("index user: %v", err)
	}
	refreshIndices(t, env.osClient.Addr())

	resp := get(t, env.srv, "/v1/search/users?q=alice")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Users []struct {
			ID       string `json:"id"`
			Username string `json:"username"`
		} `json:"users"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Users) == 0 {
		t.Fatal("expected at least 1 user, got 0")
	}
	if body.Users[0].ID != "usr_001" {
		t.Errorf("expected usr_001, got %s", body.Users[0].ID)
	}
}

func TestSearch_EmptyQueryReturnsEmpty(t *testing.T) {
	env := newTestEnv(t)
	defer env.srv.Close()

	resp := get(t, env.srv, "/v1/search/tweets?q=")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}

	var body struct {
		Tweets []any `json:"tweets"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(body.Tweets) != 0 {
		t.Errorf("expected empty tweets, got %d", len(body.Tweets))
	}
}
