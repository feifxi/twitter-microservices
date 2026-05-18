package feed_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/twitter/feed-service/internal/consumer"
	"github.com/twitter/feed-service/internal/feed"
	"github.com/twitter/feed-service/internal/tweetclient"
)

// ── stubs ─────────────────────────────────────────────────────────────────────

type stubBackend struct {
	cached    []byte
	cacheHit  bool
	ids       []string
	celebs    map[string]bool
	setCalled bool

	// snapshots keyed by tweet ID — sourced from the tweet map passed to newService.
	snapshots map[string]map[string]string

	// For You feed
	forYouExists  bool
	forYouIDs     []string
	forYouStored  []string
	userInterests map[string]float64
	userAffinity  map[string]float64
}

func (b *stubBackend) GetCachedFollowingFeed(_ context.Context, _ string) ([]byte, bool) {
	return b.cached, b.cacheHit
}
func (b *stubBackend) SetCachedFollowingFeed(_ context.Context, _ string, data []byte) {
	b.setCalled = true
	b.cached = data
}
func (b *stubBackend) GetFollowingFeed(_ context.Context, _ string, _ *string, _ int) ([]string, error) {
	entries := make([]string, len(b.ids))
	for i, id := range b.ids {
		entries[i] = consumer.EncodeTweetEntry(id)
	}
	return entries, nil
}
func (b *stubBackend) IsCeleb(_ context.Context, id string) bool { return b.celebs[id] }
func (b *stubBackend) GetTrending(_ context.Context, _ int) ([]consumer.TrendingTag, error) {
	return nil, nil
}
func (b *stubBackend) RunTweetConsumer(_ context.Context, _ *kafka.Reader) {}

func (b *stubBackend) GetUserInterests(_ context.Context, _ string, _ int) (map[string]float64, error) {
	return b.userInterests, nil
}
func (b *stubBackend) GetUserAffinity(_ context.Context, _ string, _ int) (map[string]float64, error) {
	return b.userAffinity, nil
}
func (b *stubBackend) RecommendedCacheExists(_ context.Context, _ string) bool { return b.forYouExists }
func (b *stubBackend) GetRecommendedFeed(_ context.Context, _ string, cursor *string, limit int) ([]string, error) {
	start := 0
	if cursor != nil {
		for i, id := range b.forYouIDs {
			if id == *cursor {
				start = i + 1
				break
			}
		}
	}
	if start >= len(b.forYouIDs) {
		return []string{}, nil
	}
	result := b.forYouIDs[start:]
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}
func (b *stubBackend) SetRecommendedFeed(_ context.Context, _ string, ids []string) {
	b.forYouStored = ids
	b.forYouIDs = ids
	b.forYouExists = true
}

func (b *stubBackend) PipelineTweetSnapshots(_ context.Context, ids []string) map[string]map[string]string {
	result := make(map[string]map[string]string)
	for _, id := range ids {
		if snap, ok := b.snapshots[id]; ok {
			result[id] = snap
		}
	}
	return result
}

func (b *stubBackend) PipelineTweetCounts(_ context.Context, _ []string) map[string]map[string]string {
	return map[string]map[string]string{}
}

func (b *stubBackend) PipelineUserSnapshots(_ context.Context, _ []string) map[string]map[string]string {
	return map[string]map[string]string{}
}

type stubTweets struct {
	tweets        map[string]*tweetclient.Tweet
	recent        []*tweetclient.Tweet
	popular       []*tweetclient.Tweet
	popularCalled bool
}

func (s *stubTweets) GetInteractions(_ context.Context, _ []string, _ string) (map[string]tweetclient.TweetInteraction, error) {
	return map[string]tweetclient.TweetInteraction{}, nil
}
func (s *stubTweets) GetRecentTweets(_ context.Context, _ string, _ int) ([]*tweetclient.Tweet, error) {
	return s.recent, nil
}
func (s *stubTweets) GetRecentPopularTweets(_ context.Context, _ int) ([]*tweetclient.Tweet, error) {
	s.popularCalled = true
	return s.popular, nil
}

type stubUsers struct {
	following []string
}

func (s *stubUsers) GetFollowingIDs(_ context.Context, _ string) ([]string, error) {
	return s.following, nil
}

// tweetSnapshot converts a Tweet into the Redis hash format used by PipelineTweetSnapshots.
func tweetSnapshot(t *tweetclient.Tweet) map[string]string {
	tweetType := t.Type
	if tweetType == "" {
		tweetType = "tweet"
	}
	return map[string]string{
		"author_id":  t.AuthorID,
		"body":       t.Body,
		"type":       tweetType,
		"created_at": strconv.FormatInt(t.CreatedAt.Unix(), 10),
	}
}

// snapshotsFromTweets builds a snapshot map from a tweet map — used in test setup.
func snapshotsFromTweets(tweets map[string]*tweetclient.Tweet) map[string]map[string]string {
	m := make(map[string]map[string]string, len(tweets))
	for id, t := range tweets {
		m[id] = tweetSnapshot(t)
	}
	return m
}

func newService(b *stubBackend, tw *stubTweets, us *stubUsers) *feed.Service {
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	return feed.New(b, tw, us, log)
}

func makeTweet(id string, createdAt time.Time) *tweetclient.Tweet {
	return &tweetclient.Tweet{ID: id, CreatedAt: createdAt}
}

// ── withCursor (via GetFollowingFeed) ──────────────────────────────────────────────

func TestGetFollowingFeed_UnderLimitHasNoCursor(t *testing.T) {
	now := time.Now()
	tweets := map[string]*tweetclient.Tweet{
		"tw_1": makeTweet("tw_1", now),
	}
	tw := &stubTweets{tweets: tweets}
	b := &stubBackend{ids: []string{"tw_1"}, snapshots: snapshotsFromTweets(tweets)}
	svc := newService(b, tw, &stubUsers{})

	result, cursor, err := svc.GetFollowingFeed(context.Background(), "usr_A", nil, 20)
	if err != nil {
		t.Fatalf("GetFollowingFeed: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("got %d tweets, want 1", len(result))
	}
	if cursor != nil {
		t.Errorf("next_cursor should be nil when results < limit, got %q", *cursor)
	}
}

func TestGetFollowingFeed_ExactLimitSetsCursor(t *testing.T) {
	now := time.Now()
	tweets := map[string]*tweetclient.Tweet{
		"tw_1": makeTweet("tw_1", now.Add(-2*time.Second)),
		"tw_2": makeTweet("tw_2", now.Add(-1*time.Second)),
	}
	b := &stubBackend{ids: []string{"tw_1", "tw_2"}, snapshots: snapshotsFromTweets(tweets)}
	svc := newService(b, &stubTweets{tweets: tweets}, &stubUsers{})

	prev := "tw_0"
	_, cursor, err := svc.GetFollowingFeed(context.Background(), "usr_A", &prev, 2)
	if err != nil {
		t.Fatalf("GetFollowingFeed: %v", err)
	}
	if cursor == nil {
		t.Fatal("next_cursor should be set when results == limit")
	}
	// Cursor is the encoded feed-list entry — consumer.GetFollowingFeed scans
	// the Redis list for an exact-string match, and the list stores
	// pipe-delimited entries like "T|<id>".
	if *cursor != "T|tw_2" {
		t.Errorf("next_cursor = %q, want %q", *cursor, "T|tw_2")
	}
}

// ── cache hit path ────────────────────────────────────────────────────────────

func TestGetFollowingFeed_CacheHitReturnsCachedTweets(t *testing.T) {
	now := time.Now()
	// Cache stores encoded feed-list entries — same format the Redis list
	// holds — so a cache hit can flow straight through enrichEntries.
	data, _ := json.Marshal([]string{consumer.EncodeTweetEntry("tw_cached")})

	tweet := makeTweet("tw_cached", now)
	b := &stubBackend{
		cached:    data,
		cacheHit:  true,
		snapshots: map[string]map[string]string{"tw_cached": tweetSnapshot(tweet)},
	}
	svc := newService(b, &stubTweets{tweets: map[string]*tweetclient.Tweet{"tw_cached": tweet}}, &stubUsers{})

	tweets, _, err := svc.GetFollowingFeed(context.Background(), "usr_A", nil, 20)
	if err != nil {
		t.Fatalf("GetFollowingFeed: %v", err)
	}
	if len(tweets) != 1 || tweets[0].Tweet.ID != "tw_cached" {
		t.Errorf("expected cached tweet, got %v", tweets)
	}
}

func TestGetFollowingFeed_CursorPageBypassesCache(t *testing.T) {
	now := time.Now()
	data, _ := json.Marshal([]string{"tw_cached"})

	tweets := map[string]*tweetclient.Tweet{
		"tw_db": makeTweet("tw_db", now),
	}
	b := &stubBackend{
		cached:    data,
		cacheHit:  true,
		ids:       []string{"tw_db"},
		snapshots: snapshotsFromTweets(tweets),
	}
	svc := newService(b, &stubTweets{tweets: tweets}, &stubUsers{})

	cursor := "tw_prev"
	result, _, err := svc.GetFollowingFeed(context.Background(), "usr_A", &cursor, 20)
	if err != nil {
		t.Fatalf("GetFollowingFeed: %v", err)
	}
	if len(result) != 1 || result[0].Tweet.ID != "tw_db" {
		t.Errorf("expected db tweet on cursor page, got %v", result)
	}
}

// ── celeb merge ───────────────────────────────────────────────────────────────

func TestGetFollowingFeed_CelebTweetsMergedAndSorted(t *testing.T) {
	base := time.Now()
	older := base.Add(-5 * time.Second)
	newer := base.Add(-1 * time.Second)

	tweets := map[string]*tweetclient.Tweet{
		"tw_old": makeTweet("tw_old", older),
		"tw_new": makeTweet("tw_new", newer),
	}
	b := &stubBackend{
		ids:       []string{"tw_old"},
		celebs:    map[string]bool{"usr_celeb": true},
		snapshots: snapshotsFromTweets(tweets),
	}
	tw := &stubTweets{
		tweets: tweets,
		recent: []*tweetclient.Tweet{makeTweet("tw_new", newer)},
	}
	users := &stubUsers{following: []string{"usr_celeb"}}
	svc := newService(b, tw, users)

	result, _, err := svc.GetFollowingFeed(context.Background(), "usr_A", nil, 20)
	if err != nil {
		t.Fatalf("GetFollowingFeed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 tweets after celeb merge, got %d", len(result))
	}
	if result[0].Tweet.ID != "tw_new" {
		t.Errorf("expected newer celeb tweet first, got %q", result[0].Tweet.ID)
	}
}

func TestGetFollowingFeed_NoCelebMergeOnCursorPage(t *testing.T) {
	now := time.Now()
	tweets := map[string]*tweetclient.Tweet{
		"tw_1": makeTweet("tw_1", now),
	}
	b := &stubBackend{
		ids:       []string{"tw_1"},
		celebs:    map[string]bool{"usr_celeb": true},
		snapshots: snapshotsFromTweets(tweets),
	}
	tw := &stubTweets{
		tweets: tweets,
		recent: []*tweetclient.Tweet{makeTweet("tw_celeb", now)},
	}
	users := &stubUsers{following: []string{"usr_celeb"}}
	svc := newService(b, tw, users)

	cursor := "tw_prev"
	result, _, err := svc.GetFollowingFeed(context.Background(), "usr_A", &cursor, 20)
	if err != nil {
		t.Fatalf("GetFollowingFeed: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("celeb tweets should not be merged on cursor page, got %d", len(result))
	}
}

// ── For You feed ──────────────────────────────────────────────────────────────

func makePopularTweet(id string, createdAt time.Time, likes, retweets int32, hashtags []string, authorID string) *tweetclient.Tweet {
	return &tweetclient.Tweet{ID: id, CreatedAt: createdAt, LikeCount: likes, RetweetCount: retweets, Hashtags: hashtags, AuthorID: authorID}
}

func TestGetRecommendedFeed_CacheHitSkipsRebuild(t *testing.T) {
	now := time.Now()
	tweets := map[string]*tweetclient.Tweet{
		"tw_1": makeTweet("tw_1", now),
	}
	b := &stubBackend{
		forYouExists: true,
		forYouIDs:    []string{"tw_1"},
		snapshots:    snapshotsFromTweets(tweets),
	}
	tw := &stubTweets{tweets: tweets}
	svc := newService(b, tw, &stubUsers{})

	result, _, err := svc.GetRecommendedFeed(context.Background(), "usr_A", nil, 20)
	if err != nil {
		t.Fatalf("GetRecommendedFeed: %v", err)
	}
	if len(result) != 1 {
		t.Errorf("expected 1 tweet from cache, got %d", len(result))
	}
	if tw.popularCalled {
		t.Error("GetRecentPopularTweets should not be called on cache hit")
	}
}

func TestGetRecommendedFeed_CacheMissTriggersRebuild(t *testing.T) {
	now := time.Now()
	popular := []*tweetclient.Tweet{
		makePopularTweet("tw_pop1", now.Add(-1*time.Hour), 50, 10, []string{"go"}, "usr_author"),
		makePopularTweet("tw_pop2", now.Add(-2*time.Hour), 20, 5, nil, "usr_other"),
	}
	tweets := map[string]*tweetclient.Tweet{
		"tw_pop1": makeTweet("tw_pop1", now.Add(-1*time.Hour)),
		"tw_pop2": makeTweet("tw_pop2", now.Add(-2*time.Hour)),
	}
	b := &stubBackend{forYouExists: false, snapshots: snapshotsFromTweets(tweets)}
	tw := &stubTweets{popular: popular, tweets: tweets}
	svc := newService(b, tw, &stubUsers{})

	result, _, err := svc.GetRecommendedFeed(context.Background(), "usr_A", nil, 20)
	if err != nil {
		t.Fatalf("GetRecommendedFeed: %v", err)
	}
	if len(result) != 2 {
		t.Errorf("expected 2 rebuilt tweets, got %d", len(result))
	}
	if len(b.forYouStored) != 2 {
		t.Errorf("expected SetRecommendedFeed to store 2 IDs, got %d", len(b.forYouStored))
	}
}

func TestGetRecommendedFeed_PersonalSignalsBoostScore(t *testing.T) {
	now := time.Now()
	popular := []*tweetclient.Tweet{
		makePopularTweet("tw_popular", now.Add(-1*time.Hour), 10, 5, nil, "usr_stranger"),
		makePopularTweet("tw_boosted", now.Add(-1*time.Hour), 1, 0, []string{"golang"}, "usr_fav"),
	}
	tweets := map[string]*tweetclient.Tweet{
		"tw_popular": makeTweet("tw_popular", now.Add(-1*time.Hour)),
		"tw_boosted": makeTweet("tw_boosted", now.Add(-1*time.Hour)),
	}
	b := &stubBackend{
		forYouExists:  false,
		userInterests: map[string]float64{"golang": 100},
		userAffinity:  map[string]float64{"usr_fav": 200},
		snapshots:     snapshotsFromTweets(tweets),
	}
	tw := &stubTweets{popular: popular, tweets: tweets}
	svc := newService(b, tw, &stubUsers{})

	result, _, err := svc.GetRecommendedFeed(context.Background(), "usr_A", nil, 20)
	if err != nil {
		t.Fatalf("GetRecommendedFeed: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 tweets, got %d", len(result))
	}
	if result[0].Tweet.ID != "tw_boosted" {
		t.Errorf("expected tw_boosted first (personal boost), got %q", result[0].Tweet.ID)
	}
}

func TestGetRecommendedFeed_CursorPageBypassesRebuild(t *testing.T) {
	now := time.Now()
	tweets := map[string]*tweetclient.Tweet{
		"tw_1": makeTweet("tw_1", now),
		"tw_2": makeTweet("tw_2", now.Add(-1*time.Second)),
	}
	b := &stubBackend{
		forYouExists: false,
		forYouIDs:    []string{"tw_1", "tw_2"},
		snapshots:    snapshotsFromTweets(tweets),
	}
	tw := &stubTweets{tweets: tweets}
	svc := newService(b, tw, &stubUsers{})

	cursor := "tw_1"
	result, _, err := svc.GetRecommendedFeed(context.Background(), "usr_A", &cursor, 20)
	if err != nil {
		t.Fatalf("GetRecommendedFeed: %v", err)
	}
	if len(result) != 1 || result[0].Tweet.ID != "tw_2" {
		t.Errorf("expected tw_2 on cursor page, got %v", result)
	}
	if tw.popularCalled {
		t.Error("GetRecentPopularTweets should not be called on cursor page")
	}
}
