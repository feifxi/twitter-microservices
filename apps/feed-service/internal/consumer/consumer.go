package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"github.com/twitter/shared/dlq"
	"github.com/twitter/shared/events"
	"github.com/twitter/shared/metrics"
	"github.com/twitter/shared/outbox"
	"github.com/twitter/shared/tracing"
	"golang.org/x/sync/errgroup"
)

var errUnmarshal = errors.New("consumer: malformed event payload")

const (
	dedupKey = "feed:dedup:%d"
	dedupTTL = 24 * time.Hour

	tweetSnapshotKey = "tweet:snapshot:%s"
	userSnapshotKey  = "user:snapshot:%s"
	userCountsKey    = "user:counts:%s"
	authorTweetsKey  = "author_tweets:%s"
	tweetCountsKey   = "tweet:counts:%s"

	followingKey   = "following:%s"
	followingLimit = 800
	followingTTL   = 7 * 24 * time.Hour
	celebKey       = "celeb:%s"
	celebThreshold = 1_000
	celebFlagTTL   = time.Hour

	// One ZSET per minute (tag → count). ZUNIONSTORE of the last
	// trendingWindowMinutes buckets at read time gives a sliding-window
	// leaderboard with automatic decay — old buckets expire via TTL, no
	// daily reset needed.
	trendingMinuteKey     = "trending:minute:%d"
	trendingMinuteTTL     = 70 * time.Minute
	trendingWindowMinutes = 60
	trendingWindowKey     = "trending:window:%d"
	trendingWindowKeyTTL  = 5 * time.Minute

	userInterestsKey = "user_interests:%s"
	userAffinityKey  = "user_affinity:%s"
	signalTTL        = 30 * 24 * time.Hour

	recommendedKey   = "recommended:%s"
	recommendedTSKey = "recommended_ts:%s"
	recommendedTTL   = 15 * time.Minute
)

type FanoutClient interface {
	GetFollowerIDs(ctx context.Context, userID string) ([]string, error)
}

// TweetBackfillClient is used on user.followed events to seed the new
// follower's `following:*` LIST with the followee's recent tweets — closes the
// cold-start hole where freshly-followed accounts wouldn't appear in the feed
// until they post next.
type TweetBackfillClient interface {
	GetRecentTweets(ctx context.Context, authorID string, limit int) (ids []string, err error)
}

const (
	feedEntryTweet   = "T"
	feedEntryRetweet = "R"
)

func EncodeTweetEntry(tweetID string) string {
	return feedEntryTweet + "|" + tweetID
}

func EncodeRetweetEntry(retweetID, originalTweetID, retweeterID string, createdAt int64) string {
	return feedEntryRetweet + "|" + retweetID + "|" + originalTweetID + "|" + retweeterID + "|" + strconv.FormatInt(createdAt, 10)
}

type FeedEntry struct {
	Kind        string
	RetweetID   string
	TweetID     string
	RetweeterID string
	CreatedAt   int64
}

func ParseFeedEntry(raw string) (FeedEntry, bool) {
	parts := strings.Split(raw, "|")
	if len(parts) == 0 {
		return FeedEntry{}, false
	}
	switch parts[0] {
	case feedEntryTweet:
		if len(parts) != 2 || parts[1] == "" {
			return FeedEntry{}, false
		}
		return FeedEntry{Kind: "tweet", TweetID: parts[1]}, true
	case feedEntryRetweet:
		if len(parts) != 5 {
			return FeedEntry{}, false
		}
		ts, err := strconv.ParseInt(parts[4], 10, 64)
		if err != nil {
			return FeedEntry{}, false
		}
		return FeedEntry{
			Kind:        "retweet",
			RetweetID:   parts[1],
			TweetID:     parts[2],
			RetweeterID: parts[3],
			CreatedAt:   ts,
		}, true
	}
	return FeedEntry{}, false
}

const fanoutConcurrency = 100

type Consumer struct {
	rdb       *redis.Client
	fanout    FanoutClient
	backfill  TweetBackfillClient
	dlq       *kafka.Writer
	log       *slog.Logger
	fanoutSem chan struct{}
}

func New(rdb *redis.Client, fanout FanoutClient, backfill TweetBackfillClient, log *slog.Logger) *Consumer {
	return &Consumer{
		rdb:       rdb,
		fanout:    fanout,
		backfill:  backfill,
		log:       log,
		fanoutSem: make(chan struct{}, fanoutConcurrency),
	}
}

func (c *Consumer) WithDLQ(dlq *kafka.Writer) *Consumer {
	c.dlq = dlq
	return c
}

// RunTweetConsumer guarantees at-least-once: offsets commit only after dispatch
// succeeds. The dedup key is set AFTER dispatch — claiming it first would turn
// a transient failure into a permanent lost message on redelivery.
func (c *Consumer) RunTweetConsumer(ctx context.Context, r *kafka.Reader) {
	var g errgroup.Group
	defer func() { _ = g.Wait() }()

	for {
		msg, err := r.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			c.log.ErrorContext(ctx, "kafka fetch", "topic", r.Config().Topic, "err", err)
			continue
		}

		eventID := extractEventID(msg)
		if eventID > 0 {
			seen, err := c.alreadyProcessed(ctx, eventID)
			if err != nil {
				c.log.ErrorContext(ctx, "dedup check failed; will retry", "topic", msg.Topic, "offset", msg.Offset, "err", err)
				continue
			}
			if seen {
				if err := r.CommitMessages(ctx, msg); err != nil {
					c.log.ErrorContext(ctx, "kafka commit (dup)", "topic", msg.Topic, "offset", msg.Offset, "err", err)
				}
				continue
			}
		}

		if err := c.dispatch(ctx, &g, msg); err != nil {
			metrics.KafkaConsumerErrors.WithLabelValues("feed-service", msg.Topic).Inc()
			if errors.Is(err, errUnmarshal) {
				dlq.Write(ctx, c.dlq, c.log, msg, err.Error())
				c.log.ErrorContext(ctx, "poison message; routed to DLQ", "topic", msg.Topic, "offset", msg.Offset, "err", err)
			} else {
				c.log.ErrorContext(ctx, "dispatch failed; will retry", "topic", msg.Topic, "offset", msg.Offset, "err", err)
				continue
			}
		}

		if eventID > 0 {
			if err := c.rdb.Set(ctx, fmt.Sprintf(dedupKey, eventID), 1, dedupTTL).Err(); err != nil {
				c.log.WarnContext(ctx, "dedup mark failed", "event_id", eventID, "err", err)
			}
		}
		if err := r.CommitMessages(ctx, msg); err != nil {
			c.log.ErrorContext(ctx, "kafka commit", "topic", msg.Topic, "offset", msg.Offset, "err", err)
		}
	}
}

func (c *Consumer) alreadyProcessed(ctx context.Context, eventID int64) (bool, error) {
	n, err := c.rdb.Exists(ctx, fmt.Sprintf(dedupKey, eventID)).Result()
	if err != nil {
		return false, err
	}
	return n > 0, nil
}

// Returns 0 when the header is absent or malformed.
func extractEventID(msg kafka.Message) int64 {
	for _, h := range msg.Headers {
		if h.Key == outbox.EventIDHeader {
			id, _ := strconv.ParseInt(string(h.Value), 10, 64)
			return id
		}
	}
	return 0
}

func (c *Consumer) dispatch(ctx context.Context, g *errgroup.Group, msg kafka.Message) error {
	ctx = tracing.FromKafkaMessage(ctx, msg)
	switch msg.Topic {
	case events.TopicTweetCreated:
		var evt events.TweetCreatedEvent
		if err := json.Unmarshal(msg.Value, &evt); err != nil {
			return fmt.Errorf("%w: tweet.created: %v", errUnmarshal, err)
		}
		author := c.lookupUserSnapshot(ctx, evt.AuthorID)
		c.SetTweetSnapshot(ctx, evt.TweetID, evt.AuthorID, evt.Body, "", evt.MediaID, evt.MediaURL, evt.Hashtags, evt.CreatedAt.Unix(), author)
		c.IncrementTrending(ctx, evt.Hashtags)
		// Semaphore acquired before launching — blocks here rather than inside the goroutine,
		// preventing unbounded goroutine creation under load.
		c.fanoutSem <- struct{}{}
		entry, authorID := EncodeTweetEntry(evt.TweetID), evt.AuthorID
		g.Go(func() error {
			defer func() { <-c.fanoutSem }()
			c.FanOut(ctx, entry, authorID)
			return nil
		})
		return nil

	case events.TopicTweetRetweeted:
		var evt events.TweetRetweetedEvent
		if err := json.Unmarshal(msg.Value, &evt); err != nil {
			return fmt.Errorf("%w: tweet.retweeted: %v", errUnmarshal, err)
		}
		// Reference-only — no snapshot for rt_... id; signals come from the original's snapshot.
		c.applyRetweetSignals(ctx, evt.OriginalTweetID, evt.OriginalAuthorID, evt.RetweeterID)
		c.fanoutSem <- struct{}{}
		createdAt := evt.CreatedAt.Unix()
		if createdAt <= 0 {
			createdAt = time.Now().Unix()
		}
		entry := EncodeRetweetEntry(evt.RetweetID, evt.OriginalTweetID, evt.RetweeterID, createdAt)
		retweeterID := evt.RetweeterID
		g.Go(func() error {
			defer func() { <-c.fanoutSem }()
			c.FanOut(ctx, entry, retweeterID)
			return nil
		})
		return nil

	case events.TopicTweetUnretweeted:
		var evt events.TweetUnretweetedEvent
		if err := json.Unmarshal(msg.Value, &evt); err != nil {
			return fmt.Errorf("%w: tweet.unretweeted: %v", errUnmarshal, err)
		}
		c.RemoveRetweetFromFollowers(ctx, evt.RetweeterID, evt.RetweetID)
		return nil

	case events.TopicTweetDeleted:
		var evt events.TweetDeletedEvent
		if err := json.Unmarshal(msg.Value, &evt); err != nil {
			return fmt.Errorf("%w: tweet.deleted: %v", errUnmarshal, err)
		}
		c.DeleteTweetSnapshot(ctx, evt.TweetID)
		return nil

	case events.TopicTweetLiked:
		var evt events.TweetLikedEvent
		if err := json.Unmarshal(msg.Value, &evt); err != nil {
			return fmt.Errorf("%w: tweet.liked: %v", errUnmarshal, err)
		}
		c.UpdateUserAffinity(ctx, evt.LikerID, evt.AuthorID, 1.0)
		c.UpdateUserInterests(ctx, evt.LikerID, evt.Hashtags, 1.0)
		return nil

	case events.TopicUserFollowed:
		var evt events.UserFollowedEvent
		if err := json.Unmarshal(msg.Value, &evt); err != nil {
			return fmt.Errorf("%w: user.followed: %v", errUnmarshal, err)
		}
		// Following is a strong signal — weight 5x compared to a like.
		c.UpdateUserAffinity(ctx, evt.FollowerID, evt.FolloweeID, 5.0)
		c.BackfillFollowingFeed(ctx, evt.FollowerID, evt.FolloweeID)
		return nil

	// tweet-service owns user:snapshot:{id}; feed-service only refreshes the
	// embedded author fields on tweet:snapshot:* via author_tweets:{userID}.
	case events.TopicUserUpdated:
		var evt events.UserUpdatedEvent
		if err := json.Unmarshal(msg.Value, &evt); err != nil {
			return fmt.Errorf("%w: user.updated: %v", errUnmarshal, err)
		}
		c.UpdateAuthorInTweetSnapshots(ctx, evt.UserID, evt.Username, evt.DisplayName, evt.AvatarURL)
		return nil
	}
	return nil
}

// Entry is pre-encoded by the caller ("T|<id>" or "R|<rt_id>|...").
// Celebrity authors are skipped — read path merges their tweets separately.
func (c *Consumer) FanOut(ctx context.Context, entry, authorID string) {
	celebFlagKey := fmt.Sprintf(celebKey, authorID)
	isceleb, _ := c.rdb.Exists(ctx, celebFlagKey).Result()
	if isceleb == 0 {
		// user-service writes user:counts:{id} on every follow/unfollow. Missing
		// key (HGet → redis.Nil) means count=0 — treat as non-celeb, fall through.
		count, err := c.rdb.HGet(ctx, fmt.Sprintf(userCountsKey, authorID), "follower_count").Int64()
		if err != nil && !errors.Is(err, redis.Nil) {
			c.log.ErrorContext(ctx, "get follower count", "author", authorID, "err", err)
		}
		if count >= celebThreshold {
			c.rdb.Set(ctx, celebFlagKey, "1", celebFlagTTL)
			c.log.InfoContext(ctx, "celebrity threshold reached — skipping fan-out", "author", authorID)
			return
		}
	} else {
		return
	}

	followerIDs, err := c.fanout.GetFollowerIDs(ctx, authorID)
	if err != nil {
		c.log.ErrorContext(ctx, "get follower IDs", "author", authorID, "err", err)
		return
	}

	pipe := c.rdb.Pipeline()
	for _, followerID := range followerIDs {
		key := fmt.Sprintf(followingKey, followerID)
		pipe.LPush(ctx, key, entry)
		pipe.LTrim(ctx, key, 0, followingLimit-1)
		pipe.Expire(ctx, key, followingTTL)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		c.log.ErrorContext(ctx, "pipeline fan-out", "entry", entry, "err", err)
	}
}

// Best-effort — missing snapshot skips the signal update; retweet weight is 3×.
func (c *Consumer) applyRetweetSignals(ctx context.Context, originalTweetID, originalAuthorID, retweeterID string) {
	const retweetSignalWeight = 3.0
	snap := c.GetTweetSnapshot(ctx, originalTweetID)
	if snap == nil {
		return
	}
	var hashtags []string
	if raw := snap["hashtags"]; raw != "" {
		hashtags = strings.Split(raw, ",")
	}
	if len(hashtags) > 0 {
		c.IncrementTrending(ctx, hashtags)
		c.UpdateUserInterests(ctx, retweeterID, hashtags, retweetSignalWeight)
	}
	if originalAuthorID != "" {
		c.UpdateUserAffinity(ctx, retweeterID, originalAuthorID, retweetSignalWeight)
	}
}

// O(followers × feed_length) but bounded: lists capped at followingLimit (800), runs async.
func (c *Consumer) RemoveRetweetFromFollowers(ctx context.Context, retweeterID, retweetID string) {
	followerIDs, err := c.fanout.GetFollowerIDs(ctx, retweeterID)
	if err != nil {
		c.log.ErrorContext(ctx, "get follower IDs for unretweet", "retweeter", retweeterID, "err", err)
		return
	}
	if len(followerIDs) == 0 {
		return
	}
	pipe := c.rdb.Pipeline()
	for _, followerID := range followerIDs {
		key := fmt.Sprintf(followingKey, followerID)
		// Read the whole list and re-issue an LREM by prefix match. Redis
		// LREM matches whole element strings, so we have to scan-and-delete
		// candidates locally. Done as a separate pass per follower so the
		// pipeline issues batched LREMs, not per-element round-trips.
		raws, err := c.rdb.LRange(ctx, key, 0, followingLimit-1).Result()
		if err != nil {
			c.log.WarnContext(ctx, "lrange for unretweet", "follower", followerID, "err", err)
			continue
		}
		prefix := feedEntryRetweet + "|" + retweetID + "|"
		for _, raw := range raws {
			if strings.HasPrefix(raw, prefix) {
				pipe.LRem(ctx, key, 1, raw)
			}
		}
	}
	if _, err := pipe.Exec(ctx); err != nil {
		c.log.ErrorContext(ctx, "pipeline unretweet remove", "retweeter", retweeterID, "err", err)
	}
}

// BackfillFollowingFeed seeds the new follower's LIST with the followee's
// recent tweets so they don't see an empty feed until the next post. Best-
// effort: a gRPC failure leaves the LIST as-is and the fan-out path picks up
// future tweets normally. The first-page sort by created_at in feed.Service
// puts these in the right chronological position even if the LIST has older
// entries from other followees.
const backfillLimit = 20

func (c *Consumer) BackfillFollowingFeed(ctx context.Context, followerID, followeeID string) {
	if c.backfill == nil {
		return
	}
	if c.IsCeleb(ctx, followeeID) {
		// Celebrities are merged in at read time; backfilling would duplicate.
		return
	}
	ids, err := c.backfill.GetRecentTweets(ctx, followeeID, backfillLimit)
	if err != nil {
		c.log.WarnContext(ctx, "backfill: get recent tweets", "followee", followeeID, "err", err)
		return
	}
	if len(ids) == 0 {
		return
	}
	key := fmt.Sprintf(followingKey, followerID)
	pipe := c.rdb.Pipeline()
	// LPUSH oldest-first so newest ends up at the head of the LIST — matches
	// the order fan-out maintains for live tweets.
	for i := len(ids) - 1; i >= 0; i-- {
		pipe.LPush(ctx, key, EncodeTweetEntry(ids[i]))
	}
	pipe.LTrim(ctx, key, 0, followingLimit-1)
	pipe.Expire(ctx, key, followingTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		c.log.WarnContext(ctx, "backfill: pipeline exec", "follower", followerID, "followee", followeeID, "err", err)
	}
}

func (c *Consumer) IncrementTrending(ctx context.Context, hashtags []string) {
	if len(hashtags) == 0 {
		return
	}
	key := MinuteBucketKey(MinuteBucket(time.Now()))
	pipe := c.rdb.Pipeline()
	for _, tag := range hashtags {
		pipe.ZIncrBy(ctx, key, 1, tag)
	}
	pipe.Expire(ctx, key, trendingMinuteTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		c.log.ErrorContext(ctx, "trending increment", "err", err)
	}
}

// Unions the last trendingWindowMinutes bucket ZSETs into a per-minute
// destination key (atomically overwritten by concurrent callers, TTL'd so it
// self-cleans), then server-side paginates the top N. New hashtags appear in
// the result as soon as the next read crosses ZUNIONSTORE.
func (c *Consumer) GetTrending(ctx context.Context, topN int) ([]TrendingTag, error) {
	if topN <= 0 {
		return []TrendingTag{}, nil
	}
	now := MinuteBucket(time.Now())
	keys := make([]string, trendingWindowMinutes)
	for i := 0; i < trendingWindowMinutes; i++ {
		keys[i] = MinuteBucketKey(now - int64(i))
	}
	destKey := fmt.Sprintf(trendingWindowKey, now)
	if _, err := c.rdb.ZUnionStore(ctx, destKey, &redis.ZStore{Keys: keys}).Result(); err != nil {
		return nil, err
	}
	// Best-effort — Expire on an empty key is a no-op.
	c.rdb.Expire(ctx, destKey, trendingWindowKeyTTL)

	results, err := c.rdb.ZRevRangeWithScores(ctx, destKey, 0, int64(topN-1)).Result()
	if err != nil {
		return nil, err
	}
	tags := make([]TrendingTag, len(results))
	for i, z := range results {
		tags[i] = TrendingTag{Tag: z.Member.(string), Score: int64(z.Score)}
	}
	return tags, nil
}

// Celebrity authors are NOT in this list — merged separately at read time.
func (c *Consumer) GetFollowingFeed(ctx context.Context, userID string, cursor *string, limit int) ([]string, error) {
	key := fmt.Sprintf(followingKey, userID)
	ids, err := c.rdb.LRange(ctx, key, 0, int64(followingLimit-1)).Result()
	if err != nil {
		return nil, err
	}
	start := 0
	if cursor != nil {
		found := false
		for i, id := range ids {
			if id == *cursor {
				start = i + 1
				found = true
				break
			}
		}
		if !found {
			return []string{}, nil
		}
	}
	if start >= len(ids) {
		return []string{}, nil
	}
	result := ids[start:]
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

const (
	followingCacheKey = "following_cache:%s"
	followingCacheTTL = 60 * time.Second
)

func (c *Consumer) GetCachedFollowingFeed(ctx context.Context, userID string) ([]byte, bool) {
	b, err := c.rdb.Get(ctx, fmt.Sprintf(followingCacheKey, userID)).Bytes()
	if err != nil {
		return nil, false
	}
	return b, true
}

func (c *Consumer) SetCachedFollowingFeed(ctx context.Context, userID string, data []byte) {
	c.rdb.Set(ctx, fmt.Sprintf(followingCacheKey, userID), data, followingCacheTTL)
}

// Exported for tests.
func MinuteBucket(t time.Time) int64 {
	return t.Unix() / 60
}

// Exported for tests.
func MinuteBucketKey(bucket int64) string {
	return fmt.Sprintf(trendingMinuteKey, bucket)
}

func (c *Consumer) IsCeleb(ctx context.Context, userID string) bool {
	n, _ := c.rdb.Exists(ctx, fmt.Sprintf(celebKey, userID)).Result()
	return n > 0
}

type TrendingTag struct {
	Tag   string `json:"tag"`
	Score int64  `json:"score"`
}

// Called on like or retweet. Busts the recommended-feed freshness marker so
// the next read rebuilds with the new signal instead of waiting up to 15m.
func (c *Consumer) UpdateUserInterests(ctx context.Context, userID string, hashtags []string, weight float64) {
	if len(hashtags) == 0 {
		return
	}
	key := fmt.Sprintf(userInterestsKey, userID)
	pipe := c.rdb.Pipeline()
	for _, tag := range hashtags {
		pipe.ZIncrBy(ctx, key, weight, tag)
	}
	pipe.Expire(ctx, key, signalTTL)
	pipe.Del(ctx, fmt.Sprintf(recommendedTSKey, userID))
	if _, err := pipe.Exec(ctx); err != nil {
		c.log.ErrorContext(ctx, "update user interests", "user", userID, "err", err)
	}
}

// Called on like, retweet, or follow. Busts the recommended-feed freshness
// marker — same rationale as UpdateUserInterests.
func (c *Consumer) UpdateUserAffinity(ctx context.Context, userID, authorID string, weight float64) {
	if authorID == "" {
		return
	}
	key := fmt.Sprintf(userAffinityKey, userID)
	pipe := c.rdb.Pipeline()
	pipe.ZIncrBy(ctx, key, weight, authorID)
	pipe.Expire(ctx, key, signalTTL)
	pipe.Del(ctx, fmt.Sprintf(recommendedTSKey, userID))
	if _, err := pipe.Exec(ctx); err != nil {
		c.log.ErrorContext(ctx, "update user affinity", "user", userID, "err", err)
	}
}

// Returns nil on cold start (no signals yet).
func (c *Consumer) GetUserInterests(ctx context.Context, userID string, topN int) (map[string]float64, error) {
	results, err := c.rdb.ZRevRangeWithScores(ctx, fmt.Sprintf(userInterestsKey, userID), 0, int64(topN-1)).Result()
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	m := make(map[string]float64, len(results))
	for _, z := range results {
		m[z.Member.(string)] = z.Score
	}
	return m, nil
}

// Returns nil on cold start (no signals yet).
func (c *Consumer) GetUserAffinity(ctx context.Context, userID string, topN int) (map[string]float64, error) {
	results, err := c.rdb.ZRevRangeWithScores(ctx, fmt.Sprintf(userAffinityKey, userID), 0, int64(topN-1)).Result()
	if err != nil {
		return nil, err
	}
	if len(results) == 0 {
		return nil, nil
	}
	m := make(map[string]float64, len(results))
	for _, z := range results {
		m[z.Member.(string)] = z.Score
	}
	return m, nil
}

// Returns empty when the cache has expired, signalling a rebuild is needed.
func (c *Consumer) GetRecommendedFeed(ctx context.Context, userID string, cursor *string, limit int) ([]string, error) {
	key := fmt.Sprintf(recommendedKey, userID)
	ids, err := c.rdb.LRange(ctx, key, 0, -1).Result()
	if err != nil {
		return nil, err
	}
	start := 0
	if cursor != nil {
		found := false
		for i, id := range ids {
			if id == *cursor {
				start = i + 1
				found = true
				break
			}
		}
		if !found {
			return []string{}, nil
		}
	}
	if start >= len(ids) {
		return []string{}, nil
	}
	result := ids[start:]
	if len(result) > limit {
		result = result[:limit]
	}
	return result, nil
}

// Marker is set even when tweetIDs is empty to prevent a cold-start rebuild storm.
func (c *Consumer) SetRecommendedFeed(ctx context.Context, userID string, tweetIDs []string) {
	listKey := fmt.Sprintf(recommendedKey, userID)
	tsKey := fmt.Sprintf(recommendedTSKey, userID)

	pipe := c.rdb.Pipeline()
	pipe.Del(ctx, listKey)
	if len(tweetIDs) > 0 {
		ids := make([]any, len(tweetIDs))
		for i, id := range tweetIDs {
			ids[i] = id
		}
		pipe.RPush(ctx, listKey, ids...)
	}
	// Always stamp the marker regardless of list length so Expire has a key to act on.
	pipe.Set(ctx, tsKey, "1", recommendedTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		c.log.ErrorContext(ctx, "set recommended feed", "user", userID, "err", err)
	}
}

// Dedicated marker key so an empty result set still suppresses rebuilds.
func (c *Consumer) RecommendedCacheExists(ctx context.Context, userID string) bool {
	n, _ := c.rdb.Exists(ctx, fmt.Sprintf(recommendedTSKey, userID)).Result()
	return n > 0
}

type userSnapshotFields struct {
	Username    string
	DisplayName string
	AvatarURL   string
}

func (c *Consumer) lookupUserSnapshot(ctx context.Context, userID string) userSnapshotFields {
	fields, err := c.rdb.HGetAll(ctx, fmt.Sprintf(userSnapshotKey, userID)).Result()
	if err != nil || len(fields) == 0 {
		return userSnapshotFields{}
	}
	return userSnapshotFields{
		Username:    fields["username"],
		DisplayName: fields["display_name"],
		AvatarURL:   fields["avatar_url"],
	}
}

// Counts are NOT stored here — they live in tweet:counts:{id}. Retweets do not get snapshots.
func (c *Consumer) SetTweetSnapshot(ctx context.Context, tweetID, authorID, body, replyToID, mediaID, mediaURL string, hashtags []string, createdAt int64, author userSnapshotFields) {
	tweetType := "tweet"
	if replyToID != "" {
		tweetType = "reply"
	}
	pipe := c.rdb.Pipeline()
	pipe.HSet(ctx, fmt.Sprintf(tweetSnapshotKey, tweetID),
		"author_id", authorID,
		"body", body,
		"type", tweetType,
		"reply_to_id", replyToID,
		"media_id", mediaID,
		"media_url", mediaURL,
		"hashtags", strings.Join(hashtags, ","),
		"created_at", strconv.FormatInt(createdAt, 10),
		"author_username", author.Username,
		"author_display_name", author.DisplayName,
		"author_avatar_url", author.AvatarURL,
	)
	pipe.SAdd(ctx, fmt.Sprintf(authorTweetsKey, authorID), tweetID)
	if _, err := pipe.Exec(ctx); err != nil {
		c.log.ErrorContext(ctx, "set tweet snapshot", "tweet_id", tweetID, "err", err)
	}
}

// Also removes the tweet from the author's author_tweets set.
func (c *Consumer) DeleteTweetSnapshot(ctx context.Context, tweetID string) {
	key := fmt.Sprintf(tweetSnapshotKey, tweetID)
	// Read author_id before deleting so we can clean up the reverse index.
	authorID, _ := c.rdb.HGet(ctx, key, "author_id").Result()
	pipe := c.rdb.Pipeline()
	pipe.Del(ctx, key)
	if authorID != "" {
		pipe.SRem(ctx, fmt.Sprintf(authorTweetsKey, authorID), tweetID)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		c.log.ErrorContext(ctx, "delete tweet snapshot", "tweet_id", tweetID, "err", err)
	}
}

// Keeps embedded author fields in tweet:snapshot:* eventually consistent.
func (c *Consumer) UpdateAuthorInTweetSnapshots(ctx context.Context, userID, username, displayName, avatarURL string) {
	tweetIDs, err := c.rdb.SMembers(ctx, fmt.Sprintf(authorTweetsKey, userID)).Result()
	if err != nil || len(tweetIDs) == 0 {
		return
	}
	pipe := c.rdb.Pipeline()
	for _, id := range tweetIDs {
		pipe.HSet(ctx, fmt.Sprintf(tweetSnapshotKey, id),
			"author_username", username,
			"author_display_name", displayName,
			"author_avatar_url", avatarURL,
		)
	}
	if _, err := pipe.Exec(ctx); err != nil {
		c.log.ErrorContext(ctx, "update author in tweet snapshots", "user_id", userID, "err", err)
	}
}

// Returns nil when the tweet is deleted or not yet cached.
func (c *Consumer) GetTweetSnapshot(ctx context.Context, tweetID string) map[string]string {
	fields, err := c.rdb.HGetAll(ctx, fmt.Sprintf(tweetSnapshotKey, tweetID)).Result()
	if err != nil || len(fields) == 0 {
		return nil
	}
	return fields
}

// Returns nil on miss.
func (c *Consumer) GetUserSnapshot(ctx context.Context, userID string) map[string]string {
	fields, err := c.rdb.HGetAll(ctx, fmt.Sprintf(userSnapshotKey, userID)).Result()
	if err != nil || len(fields) == 0 {
		return nil
	}
	return fields
}

func (c *Consumer) PipelineTweetSnapshots(ctx context.Context, ids []string) map[string]map[string]string {
	type entry struct {
		id  string
		cmd *redis.MapStringStringCmd
	}
	cmds := make([]entry, 0, len(ids))
	pipe := c.rdb.Pipeline()
	for _, id := range ids {
		cmds = append(cmds, entry{id: id, cmd: pipe.HGetAll(ctx, fmt.Sprintf(tweetSnapshotKey, id))})
	}
	pipe.Exec(ctx) //nolint:errcheck — individual cmd errors checked below
	result := make(map[string]map[string]string, len(ids))
	for _, e := range cmds {
		if fields, err := e.cmd.Result(); err == nil && len(fields) > 0 {
			result[e.id] = fields
		}
	}
	return result
}

// Missing keys are absent from the result (caller treats as zero-value).
func (c *Consumer) PipelineUserSnapshots(ctx context.Context, ids []string) map[string]map[string]string {
	type entry struct {
		id  string
		cmd *redis.MapStringStringCmd
	}
	cmds := make([]entry, 0, len(ids))
	pipe := c.rdb.Pipeline()
	for _, id := range ids {
		cmds = append(cmds, entry{id: id, cmd: pipe.HGetAll(ctx, fmt.Sprintf(userSnapshotKey, id))})
	}
	pipe.Exec(ctx) //nolint:errcheck — individual cmd errors checked below
	result := make(map[string]map[string]string, len(ids))
	for _, e := range cmds {
		if fields, err := e.cmd.Result(); err == nil && len(fields) > 0 {
			result[e.id] = fields
		}
	}
	return result
}

// Missing keys return an empty map entry (counts treated as zero).
func (c *Consumer) PipelineTweetCounts(ctx context.Context, ids []string) map[string]map[string]string {
	type entry struct {
		id  string
		cmd *redis.MapStringStringCmd
	}
	cmds := make([]entry, 0, len(ids))
	pipe := c.rdb.Pipeline()
	for _, id := range ids {
		cmds = append(cmds, entry{id: id, cmd: pipe.HGetAll(ctx, fmt.Sprintf(tweetCountsKey, id))})
	}
	pipe.Exec(ctx) //nolint:errcheck — individual cmd errors checked below
	result := make(map[string]map[string]string, len(ids))
	for _, e := range cmds {
		if fields, err := e.cmd.Result(); err == nil && len(fields) > 0 {
			result[e.id] = fields
		}
	}
	return result
}

