package feed

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/twitter/feed-service/internal/consumer"
	"github.com/twitter/feed-service/internal/tweetclient"
)

// FeedItem is the discriminated union returned by feed endpoints.
// Kind == "tweet"   → Tweet is set, Retweet is nil.
// Kind == "retweet" → Tweet is the canonical ORIGINAL, Retweet wraps the
// thin retweet record (id + retweeter + when).
type FeedItem struct {
	Kind    string             `json:"kind"`
	Tweet   *tweetclient.Tweet `json:"tweet"`
	Retweet *RetweetWrapper    `json:"retweet"`
}

// RetweetWrapper carries the rt_... record metadata. The original tweet is
// transported separately via FeedItem.Tweet.
type RetweetWrapper struct {
	ID        string             `json:"id"`
	Retweeter *tweetclient.Author `json:"retweeter"`
	CreatedAt time.Time          `json:"created_at"`
}

// Static content and counts come from Redis snapshots via feedBackend.
type TweetFetcher interface {
	GetInteractions(ctx context.Context, tweetIDs []string, viewerID string) (map[string]tweetclient.TweetInteraction, error)
	GetRecentTweets(ctx context.Context, authorID string, limit int) ([]*tweetclient.Tweet, error)
	GetRecentPopularTweets(ctx context.Context, limit int) ([]*tweetclient.Tweet, error)
}

type UserClient interface {
	GetFollowingIDs(ctx context.Context, userID string) ([]string, error)
}

// Read-path only — RunTweetConsumer is wired separately in cmd/main.go.
type feedBackend interface {
	GetCachedFollowingFeed(ctx context.Context, userID string) ([]byte, bool)
	SetCachedFollowingFeed(ctx context.Context, userID string, data []byte)
	GetFollowingFeed(ctx context.Context, userID string, cursor *string, limit int) ([]string, error)
	IsCeleb(ctx context.Context, userID string) bool
	GetTrending(ctx context.Context, topN int) ([]consumer.TrendingTag, error)
	PipelineTweetSnapshots(ctx context.Context, ids []string) map[string]map[string]string
	PipelineTweetCounts(ctx context.Context, ids []string) map[string]map[string]string
	PipelineUserSnapshots(ctx context.Context, ids []string) map[string]map[string]string

	GetUserInterests(ctx context.Context, userID string, topN int) (map[string]float64, error)
	GetUserAffinity(ctx context.Context, userID string, topN int) (map[string]float64, error)
	RecommendedCacheExists(ctx context.Context, userID string) bool
	GetRecommendedFeed(ctx context.Context, userID string, cursor *string, limit int) ([]string, error)
	SetRecommendedFeed(ctx context.Context, userID string, tweetIDs []string)
}

type Service struct {
	consumer feedBackend
	tweets   TweetFetcher
	users    UserClient
	log      *slog.Logger
}

func New(c feedBackend, tweets TweetFetcher, users UserClient, log *slog.Logger) *Service {
	return &Service{consumer: c, tweets: tweets, users: users, log: log}
}

func (s *Service) GetFollowingFeed(ctx context.Context, userID string, cursor *string, limit int) ([]FeedItem, *string, error) {
	firstPage := cursor == nil

	// Use ID cache only for the first page — cursor requests bypass it.
	var entries []string
	if firstPage {
		if cached, ok := s.consumer.GetCachedFollowingFeed(ctx, userID); ok {
			var cachedEntries []string
			if json.Unmarshal(cached, &cachedEntries) == nil {
				entries = cachedEntries
			}
		}
	}

	if entries == nil {
		raw, err := s.consumer.GetFollowingFeed(ctx, userID, cursor, limit)
		if err != nil {
			return nil, nil, err
		}
		entries = raw

		// Merge celeb tweets only on first page. Celebs' content arrives as
		// "T|..." entries inline — no separate code path required at read time.
		if firstPage {
			celebEntries := s.fetchCelebFeedEntries(ctx, userID)
			entries = mergeDedupe(entries, celebEntries)
			if b, err := json.Marshal(entries); err == nil {
				s.consumer.SetCachedFollowingFeed(ctx, userID, b)
			}
		}
	}

	// Cap entries to avoid enriching far more than we need.
	if len(entries) > limit {
		entries = entries[:limit]
	}

	items := s.enrichEntries(ctx, entries, userID)
	if items == nil {
		items = []FeedItem{}
	}

	// On the first page, celeb entries may have been appended out of order —
	// sort by the entry's effective timestamp so the timeline is chronological.
	if firstPage {
		sort.Slice(items, func(i, j int) bool {
			return itemSortAt(items[i]).After(itemSortAt(items[j]))
		})
	}

	// Cursor is the last consumed Redis-list entry — encoded form, since
	// consumer.GetFollowingFeed scans the list for an exact-string match. We
	// base it on len(entries), not len(items), so a page containing
	// deleted-original entries still produces a cursor.
	return items, cursorAt(entries, limit), nil
}

func (s *Service) GetRecommendedFeed(ctx context.Context, userID string, cursor *string, limit int) ([]FeedItem, *string, error) {
	// Rebuild only on first page when cache is expired.
	if cursor == nil && !s.consumer.RecommendedCacheExists(ctx, userID) {
		if err := s.rebuildRecommendedFeed(ctx, userID); err != nil {
			s.log.ErrorContext(ctx, "rebuild recommended feed", "user", userID, "err", err)
		}
	}

	ids, err := s.consumer.GetRecommendedFeed(ctx, userID, cursor, limit)
	if err != nil {
		return nil, nil, err
	}

	// Recommended feed stores raw tweet IDs (originals). Wrap each as a
	// "T|<id>" entry so enrichEntries shares the same code path as following.
	entries := make([]string, len(ids))
	for i, id := range ids {
		entries[i] = consumer.EncodeTweetEntry(id)
	}
	items := s.enrichEntries(ctx, entries, userID)
	if items == nil {
		items = []FeedItem{}
	}
	// Cursor for recommended feed is a RAW tweet ID — consumer.GetRecommendedFeed
	// scans `recommended:{userID}` (which stores raw IDs) for an exact match.
	// Using the encoded entry here would never match, breaking pagination.
	return items, cursorAt(ids, limit), nil
}

// cursorAt returns a pointer to source[limit-1] when source has at least
// `limit` elements, signalling "this page was full, more may follow." Source
// elements must match what the corresponding consumer accessor scans for —
// encoded entries for the following list, raw IDs for the recommended list.
func cursorAt(source []string, limit int) *string {
	if len(source) < limit {
		return nil
	}
	cur := source[limit-1]
	return &cur
}

// enrichEntries parses each feed-list entry, batches Redis fetches for tweet
// snapshots, counts, viewer interactions, and (for retweets) retweeter user
// snapshots, then assembles []FeedItem. Originals with missing snapshots
// (deleted tweets) drop out naturally.
func (s *Service) enrichEntries(ctx context.Context, entries []string, viewerID string) []FeedItem {
	if len(entries) == 0 {
		return nil
	}

	parsed := make([]consumer.FeedEntry, 0, len(entries))
	tweetIDSet := make(map[string]struct{}, len(entries))
	retweeterIDSet := make(map[string]struct{})
	for _, raw := range entries {
		e, ok := consumer.ParseFeedEntry(raw)
		if !ok {
			continue
		}
		parsed = append(parsed, e)
		tweetIDSet[e.TweetID] = struct{}{}
		if e.Kind == "retweet" {
			retweeterIDSet[e.RetweeterID] = struct{}{}
		}
	}
	if len(parsed) == 0 {
		return nil
	}
	tweetIDs := setToSlice(tweetIDSet)
	retweeterIDs := setToSlice(retweeterIDSet)

	var (
		tweetSnaps   map[string]map[string]string
		counts       map[string]map[string]string
		interactions map[string]tweetclient.TweetInteraction
		userSnaps    map[string]map[string]string
		wg           sync.WaitGroup
	)
	wg.Add(4)
	go func() {
		defer wg.Done()
		tweetSnaps = s.consumer.PipelineTweetSnapshots(ctx, tweetIDs)
	}()
	go func() {
		defer wg.Done()
		counts = s.consumer.PipelineTweetCounts(ctx, tweetIDs)
	}()
	go func() {
		defer wg.Done()
		if viewerID == "" {
			return
		}
		var err error
		interactions, err = s.tweets.GetInteractions(ctx, tweetIDs, viewerID)
		if err != nil {
			s.log.ErrorContext(ctx, "get interactions", "err", err)
		}
	}()
	go func() {
		defer wg.Done()
		if len(retweeterIDs) == 0 {
			return
		}
		userSnaps = s.consumer.PipelineUserSnapshots(ctx, retweeterIDs)
	}()
	wg.Wait()

	tweetCache := make(map[string]*tweetclient.Tweet, len(tweetIDs))
	for id, snap := range tweetSnaps {
		t := snapshotToTweet(id, snap)
		if c, ok := counts[id]; ok {
			t.LikeCount = parseInt32(c["like_count"])
			t.RetweetCount = parseInt32(c["retweet_count"])
			t.ReplyCount = parseInt32(c["reply_count"])
		}
		if interactions != nil {
			if ia, ok := interactions[id]; ok {
				t.IsLiked = ia.IsLiked
				t.IsRetweeted = ia.IsRetweeted
			}
		}
		tweetCache[id] = t
	}

	items := make([]FeedItem, 0, len(parsed))
	for _, e := range parsed {
		t, ok := tweetCache[e.TweetID]
		if !ok {
			// Original was deleted — skip the feed entry entirely.
			continue
		}
		if e.Kind == "retweet" {
			items = append(items, FeedItem{
				Kind:  "retweet",
				Tweet: t,
				Retweet: &RetweetWrapper{
					ID:        e.RetweetID,
					Retweeter: userSnapshotToAuthor(e.RetweeterID, userSnaps[e.RetweeterID]),
					CreatedAt: time.Unix(e.CreatedAt, 0).UTC(),
				},
			})
			continue
		}
		items = append(items, FeedItem{Kind: "tweet", Tweet: t})
	}
	return items
}

func setToSlice(set map[string]struct{}) []string {
	out := make([]string, 0, len(set))
	for k := range set {
		if k != "" {
			out = append(out, k)
		}
	}
	return out
}

func userSnapshotToAuthor(id string, snap map[string]string) *tweetclient.Author {
	if snap == nil {
		return &tweetclient.Author{ID: id}
	}
	var avatar *string
	if v := snap["avatar_url"]; v != "" {
		avatar = &v
	}
	return &tweetclient.Author{
		ID:          id,
		Username:    snap["username"],
		DisplayName: snap["display_name"],
		AvatarURL:   avatar,
	}
}

func parseInt32(s string) int32 {
	if s == "" {
		return 0
	}
	v, _ := strconv.ParseInt(s, 10, 32)
	return int32(v)
}

// Author fields are embedded as author_username/display_name/avatar_url.
func snapshotToTweet(id string, f map[string]string) *tweetclient.Tweet {
	t := &tweetclient.Tweet{
		ID:       id,
		AuthorID: f["author_id"],
		Body:     f["body"],
		Type:     f["type"],
	}
	if t.Type == "" {
		t.Type = "tweet"
	}
	if v := f["reply_to_id"]; v != "" {
		t.ReplyToID = &v
	}
	if v := f["media_id"]; v != "" {
		t.MediaID = &v
	}
	if v := f["media_url"]; v != "" {
		t.MediaURL = &v
	}
	if v := f["hashtags"]; v != "" {
		t.Hashtags = strings.Split(v, ",")
	}
	if v := f["created_at"]; v != "" {
		if ts, err := strconv.ParseInt(v, 10, 64); err == nil {
			t.CreatedAt = time.Unix(ts, 0).UTC()
		}
	}
	if t.AuthorID != "" {
		var avatarURL *string
		if v := f["author_avatar_url"]; v != "" {
			avatarURL = &v
		}
		t.Author = &tweetclient.Author{
			ID:          t.AuthorID,
			Username:    f["author_username"],
			DisplayName: f["author_display_name"],
			AvatarURL:   avatarURL,
		}
	}
	return t
}

// itemSortAt returns the effective sort timestamp for an item — the retweet
// time for retweets, the original tweet's created_at otherwise.
func itemSortAt(item FeedItem) time.Time {
	if item.Kind == "retweet" && item.Retweet != nil {
		return item.Retweet.CreatedAt
	}
	if item.Tweet != nil {
		return item.Tweet.CreatedAt
	}
	return time.Time{}
}

func (s *Service) GetTrending(ctx context.Context, limit int) ([]consumer.TrendingTag, error) {
	return s.consumer.GetTrending(ctx, limit)
}

// Stores ranked IDs in recommended:{userID} with a 15-min TTL.
func (s *Service) rebuildRecommendedFeed(ctx context.Context, userID string) error {
	candidates, err := s.tweets.GetRecentPopularTweets(ctx, 100)
	if err != nil {
		return err
	}

	interests, err := s.consumer.GetUserInterests(ctx, userID, 50)
	if err != nil {
		s.log.ErrorContext(ctx, "get user interests for recommended rebuild", "user", userID, "err", err)
	}
	affinity, err := s.consumer.GetUserAffinity(ctx, userID, 50)
	if err != nil {
		s.log.ErrorContext(ctx, "get user affinity for recommended rebuild", "user", userID, "err", err)
	}

	type scored struct {
		id    string
		score float64
	}
	ranked := make([]scored, 0, len(candidates))
	now := time.Now()

	for _, t := range candidates {
		hoursOld := now.Sub(t.CreatedAt).Hours()
		base := float64(int(t.LikeCount)+int(t.RetweetCount)*2+int(t.ReplyCount)) / math.Max(math.Pow(hoursOld, 1.5), 0.1)

		var topicBoost float64
		for _, tag := range t.Hashtags {
			topicBoost += interests[tag]
		}
		authorBoost := affinity[t.AuthorID]

		ranked = append(ranked, scored{
			id:    t.ID,
			score: base + topicBoost*0.4 + authorBoost*0.3,
		})
	}

	sort.Slice(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })

	ids := make([]string, len(ranked))
	for i, r := range ranked {
		ids[i] = r.id
	}
	s.consumer.SetRecommendedFeed(ctx, userID, ids)
	return nil
}

// Returns pre-encoded "T|..." entries so the read path treats them identically to fan-out entries.
func (s *Service) fetchCelebFeedEntries(ctx context.Context, viewerID string) []string {
	followingIDs, err := s.users.GetFollowingIDs(ctx, viewerID)
	if err != nil {
		s.log.ErrorContext(ctx, "get following ids for celeb merge", "user", viewerID, "err", err)
		return nil
	}
	var mu sync.Mutex
	var wg sync.WaitGroup
	var result []string
	for _, followeeID := range followingIDs {
		if !s.consumer.IsCeleb(ctx, followeeID) {
			continue
		}
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			celebTweets, err := s.tweets.GetRecentTweets(ctx, id, 20)
			if err != nil {
				s.log.ErrorContext(ctx, "fetch celeb tweets", "author", id, "err", err)
				return
			}
			entries := make([]string, len(celebTweets))
			for i, t := range celebTweets {
				entries[i] = consumer.EncodeTweetEntry(t.ID)
			}
			mu.Lock()
			result = append(result, entries...)
			mu.Unlock()
		}(followeeID)
	}
	wg.Wait()
	return result
}

func mergeDedupe(a, b []string) []string {
	seen := make(map[string]struct{}, len(a))
	out := make([]string, 0, len(a)+len(b))
	for _, id := range a {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	for _, id := range b {
		if _, ok := seen[id]; !ok {
			seen[id] = struct{}{}
			out = append(out, id)
		}
	}
	return out
}
