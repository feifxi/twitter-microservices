package consumer

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

// stubFanout implements FanoutClient for tests.
type stubFanout struct {
	followerIDs   []string
	followerCount int64
}

func (s *stubFanout) GetFollowerIDs(_ context.Context, _ string) ([]string, error) {
	return s.followerIDs, nil
}
func (s *stubFanout) GetFollowerCount(_ context.Context, _ string) (int64, error) {
	return s.followerCount, nil
}

func newTestConsumer(t *testing.T, fanout FanoutClient) (*Consumer, *miniredis.Miniredis) {
	t.Helper()
	mr := miniredis.RunT(t)
	rdb := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { rdb.Close() })
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	return New(rdb, fanout, log), mr
}

func TestFanOut_PushesToFollowerTimelines(t *testing.T) {
	fanout := &stubFanout{followerIDs: []string{"usr_A", "usr_B"}, followerCount: 2}
	c, mr := newTestConsumer(t, fanout)

	c.FanOut(context.Background(), "tw_001", "usr_author")

	for _, follower := range []string{"usr_A", "usr_B"} {
		key := fmt.Sprintf(followingKey, follower)
		ids, err := mr.List(key)
		if err != nil {
			t.Fatalf("miniredis List(%s): %v", key, err)
		}
		if len(ids) != 1 || ids[0] != "tw_001" {
			t.Errorf("feed for %s: got %v, want [tw_001]", follower, ids)
		}
	}
}

func TestFanOut_SkipsCeleb(t *testing.T) {
	fanout := &stubFanout{followerIDs: []string{"usr_A"}, followerCount: int64(celebThreshold + 1)}
	c, mr := newTestConsumer(t, fanout)

	c.FanOut(context.Background(), "tw_001", "usr_celeb")

	// No feed entries should be written
	key := fmt.Sprintf(followingKey, "usr_A")
	ids, _ := mr.List(key)
	if len(ids) != 0 {
		t.Errorf("expected no fan-out for celeb, got %v", ids)
	}
	// Celeb flag should be set
	if !c.IsCeleb(context.Background(), "usr_celeb") {
		t.Error("expected celeb flag to be set")
	}
}

func TestFanOut_TrimsTimeline(t *testing.T) {
	fanout := &stubFanout{followerIDs: []string{"usr_A"}, followerCount: 1}
	c, _ := newTestConsumer(t, fanout)
	ctx := context.Background()

	// Push followingLimit + 5 tweets
	for i := range followingLimit + 5 {
		c.FanOut(ctx, fmt.Sprintf("tw_%04d", i), "usr_author")
	}

	ids, _ := c.GetFollowingFeed(ctx, "usr_A", nil, followingLimit+10)
	if len(ids) > followingLimit {
		t.Errorf("expected feed capped at %d, got %d", followingLimit, len(ids))
	}
}

func TestIncrementTrending_UpdatesLeaderboard(t *testing.T) {
	fanout := &stubFanout{}
	c, _ := newTestConsumer(t, fanout)
	ctx := context.Background()

	c.IncrementTrending(ctx, []string{"#golang", "#go", "#golang"})
	c.IncrementTrending(ctx, []string{"#golang"})

	tags, err := c.GetTrending(ctx, 10)
	if err != nil {
		t.Fatalf("GetTrending: %v", err)
	}
	if len(tags) == 0 {
		t.Fatal("expected trending tags")
	}
	if tags[0].Tag != "#golang" {
		t.Errorf("expected #golang at top, got %q", tags[0].Tag)
	}
	if tags[0].Score != 3 {
		t.Errorf("expected #golang score 3, got %d", tags[0].Score)
	}
}

func TestGetTrending_RespectsLimit(t *testing.T) {
	fanout := &stubFanout{}
	c, _ := newTestConsumer(t, fanout)
	ctx := context.Background()

	for i := range 20 {
		c.IncrementTrending(ctx, []string{fmt.Sprintf("#tag%d", i)})
	}

	tags, err := c.GetTrending(ctx, 5)
	if err != nil {
		t.Fatalf("GetTrending: %v", err)
	}
	if len(tags) != 5 {
		t.Errorf("expected 5 tags, got %d", len(tags))
	}
}

func TestMinuteBucket_Stable(t *testing.T) {
	base := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	b1 := MinuteBucket(base)
	b2 := MinuteBucket(base.Add(30 * time.Second))
	if b1 != b2 {
		t.Errorf("same minute should produce same bucket: %d vs %d", b1, b2)
	}

	b3 := MinuteBucket(base.Add(61 * time.Second))
	if b1 == b3 {
		t.Error("different minutes should produce different buckets")
	}
}

func TestBucketKey_Format(t *testing.T) {
	key := BucketKey("#golang", 12345)
	expected := "trending:#golang:12345"
	if key != expected {
		t.Errorf("BucketKey = %q, want %q", key, expected)
	}
}
