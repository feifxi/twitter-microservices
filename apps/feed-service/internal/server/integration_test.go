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
	"github.com/redis/go-redis/v9"
	"github.com/twitter/feed-service/internal/consumer"
	"github.com/twitter/feed-service/internal/feed"
	"github.com/twitter/feed-service/internal/server"
	"github.com/twitter/feed-service/internal/tweetclient"
)

// stubFanout satisfies consumer.FanoutClient.
type stubFanout struct{}

func (stubFanout) GetFollowerIDs(_ context.Context, _ string) ([]string, error) { return nil, nil }

// stubTweets satisfies feed.TweetFetcher.
type stubTweets struct{}

func (stubTweets) GetInteractions(_ context.Context, _ []string, _ string) (map[string]tweetclient.TweetInteraction, error) {
	return map[string]tweetclient.TweetInteraction{}, nil
}
func (stubTweets) GetRecentTweets(_ context.Context, _ string, _ int) ([]*tweetclient.Tweet, error) {
	return nil, nil
}
func (stubTweets) GetRecentPopularTweets(_ context.Context, _ int) ([]*tweetclient.Tweet, error) {
	return nil, nil
}

// stubUsers satisfies feed.UserClient.
type stubUsers struct{}

func (stubUsers) GetFollowingIDs(_ context.Context, _ string) ([]string, error) { return nil, nil }

func newTestServer(t *testing.T) (*httptest.Server, *consumer.Consumer) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })

	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	os.Setenv("SERVICE_TOKEN", "test-service-token")

	c := consumer.New(rdb, stubFanout{}, nil, log)
	svc := feed.New(c, stubTweets{}, stubUsers{}, log)
	srv := server.New(svc, log)

	return httptest.NewServer(srv.Handler()), c
}

func withAuth(req *http.Request) *http.Request {
	req.Header.Set("X-User-ID", "usr_test")
	req.Header.Set("X-User-Email", "test@example.com")
	req.Header.Set("X-User-Username", "testuser")
	return req
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestIntegration_Healthz(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/healthz")
	if err != nil {
		t.Fatalf("healthz: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestIntegration_Following_RequiresAuth(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/feed/following")
	if err != nil {
		t.Fatalf("feed: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}

func TestIntegration_Following_Empty(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	req, _ := http.NewRequest(http.MethodGet, ts.URL+"/v1/feed/following", nil)
	resp, err := http.DefaultClient.Do(withAuth(req))
	if err != nil {
		t.Fatalf("feed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]any
	json.NewDecoder(resp.Body).Decode(&body)
	if body["items"] == nil {
		t.Error("expected items field in response")
	}
}

func TestIntegration_Trending_NoAuth(t *testing.T) {
	ts, _ := newTestServer(t)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/v1/feed/trending")
	if err != nil {
		t.Fatalf("trending: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
}

func TestIntegration_Trending_WithTags(t *testing.T) {
	ts, c := newTestServer(t)
	defer ts.Close()

	c.IncrementTrending(context.Background(), []string{"#golang", "#microservices"})
	c.IncrementTrending(context.Background(), []string{"#golang"})

	resp, err := http.Get(ts.URL + "/v1/feed/trending?limit=5")
	if err != nil {
		t.Fatalf("trending: %v", err)
	}
	defer resp.Body.Close()

	var body struct {
		Trending []struct {
			Tag   string `json:"tag"`
			Score int64  `json:"score"`
		} `json:"trending"`
	}
	json.NewDecoder(resp.Body).Decode(&body)
	if len(body.Trending) == 0 {
		t.Fatal("expected trending tags")
	}
	if body.Trending[0].Tag != "#golang" {
		t.Errorf("expected #golang at top, got %q", body.Trending[0].Tag)
	}
	if body.Trending[0].Score != 2 {
		t.Errorf("expected score 2, got %d", body.Trending[0].Score)
	}
}
