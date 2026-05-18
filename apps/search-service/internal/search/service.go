package search

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/twitter/search-service/internal/opensearch"
)

const (
	ModeKeyword  = "keyword"
	ModeSemantic = "semantic"
	ModeHybrid   = "hybrid"

	defaultLimit = 20
	maxLimit     = 100
)

type TweetResult struct {
	Doc         opensearch.TweetDoc
	Author      AuthorSnapshot
	Counts      TweetCounts
	IsLiked     bool
	IsRetweeted bool
}

type AuthorSnapshot struct {
	Username    string
	DisplayName string
	AvatarURL   string
}

type TweetCounts struct {
	LikeCount    int32
	RetweetCount int32
	ReplyCount   int32
}

type TweetInteraction struct {
	IsLiked     bool
	IsRetweeted bool
}

type UserResult struct {
	Doc           opensearch.UserDoc
	FollowerCount int64
	IsFollowing   bool
}

type TweetSearcher interface {
	SearchTweets(ctx context.Context, query string, vector []float32, mode string, from, size int) ([]opensearch.TweetDoc, error)
	SearchUsers(ctx context.Context, query string, from, size int) ([]opensearch.UserDoc, error)
}

type Embedder interface {
	Embed(ctx context.Context, text string) ([]float32, error)
}

type UserEnricher interface {
	BatchGetFollowerCounts(ctx context.Context, userIDs []string) (map[string]int64, error)
	GetFollowState(ctx context.Context, viewerID string, targetIDs []string) (map[string]bool, error)
}

type TweetEnricher interface {
	BatchAuthorSnapshots(ctx context.Context, authorIDs []string) map[string]AuthorSnapshot
	BatchTweetCounts(ctx context.Context, tweetIDs []string) map[string]TweetCounts
	GetInteractions(ctx context.Context, viewerID string, tweetIDs []string) (map[string]TweetInteraction, error)
}

type Service struct {
	os      TweetSearcher
	embed   Embedder
	users   UserEnricher
	tweets  TweetEnricher
	log     *slog.Logger
}

func New(os TweetSearcher, embed Embedder, users UserEnricher, tweets TweetEnricher, log *slog.Logger) *Service {
	return &Service{os: os, embed: embed, users: users, tweets: tweets, log: log}
}

// Third return is the effective mode — differs from requested when embedding fails and falls back to keyword.
func (s *Service) SearchTweets(ctx context.Context, query, mode, viewerID string, cursor *string, limit int) ([]TweetResult, *string, string, error) {
	limit = clampLimit(limit)
	from := decodeCursor(cursor)
	mode = normaliseMode(mode)
	effectiveMode := mode

	var vector []float32
	if mode == ModeSemantic || mode == ModeHybrid {
		vec, err := s.embed.Embed(ctx, query)
		if err != nil {
			s.log.ErrorContext(ctx, "embed query — falling back to keyword", "mode", mode, "err", err)
			effectiveMode = ModeKeyword
		} else {
			vector = vec
		}
	}

	docs, err := s.os.SearchTweets(ctx, query, vector, effectiveMode, from, limit)
	if err != nil {
		return nil, nil, effectiveMode, err
	}

	results := s.enrichTweets(ctx, docs, viewerID)
	nextCursor := nextCursorFor(from, limit, len(docs))
	return results, nextCursor, effectiveMode, nil
}

func (s *Service) enrichTweets(ctx context.Context, docs []opensearch.TweetDoc, viewerID string) []TweetResult {
	if len(docs) == 0 {
		return []TweetResult{}
	}

	tweetIDs := make([]string, len(docs))
	authorIDs := make([]string, 0, len(docs))
	seenAuthor := make(map[string]struct{}, len(docs))
	for i, d := range docs {
		tweetIDs[i] = d.ID
		if _, ok := seenAuthor[d.AuthorID]; !ok && d.AuthorID != "" {
			authorIDs = append(authorIDs, d.AuthorID)
			seenAuthor[d.AuthorID] = struct{}{}
		}
	}

	authors := s.tweets.BatchAuthorSnapshots(ctx, authorIDs)
	counts := s.tweets.BatchTweetCounts(ctx, tweetIDs)

	var interactions map[string]TweetInteraction
	if viewerID != "" {
		var err error
		interactions, err = s.tweets.GetInteractions(ctx, viewerID, tweetIDs)
		if err != nil {
			s.log.ErrorContext(ctx, "search tweets: get interactions — falling back to false", "err", err)
			interactions = map[string]TweetInteraction{}
		}
	}

	out := make([]TweetResult, len(docs))
	for i, d := range docs {
		c := counts[d.ID]
		// Fall back to OpenSearch denormalized counts when Redis cold
		if _, ok := counts[d.ID]; !ok {
			c = TweetCounts{LikeCount: d.LikeCount, RetweetCount: d.RetweetCount}
		}
		out[i] = TweetResult{
			Doc:         d,
			Author:      authors[d.AuthorID],
			Counts:      c,
			IsLiked:     interactions[d.ID].IsLiked,
			IsRetweeted: interactions[d.ID].IsRetweeted,
		}
	}
	return out
}

// keyword-only — semantic user search adds marginal value
func (s *Service) SearchUsers(ctx context.Context, query, viewerID string, cursor *string, limit int) ([]UserResult, *string, error) {
	limit = clampLimit(limit)
	from := decodeCursor(cursor)

	docs, err := s.os.SearchUsers(ctx, query, from, limit)
	if err != nil {
		return nil, nil, err
	}

	results := s.enrichUsers(ctx, docs, viewerID)
	nextCursor := nextCursorFor(from, limit, len(docs))
	return results, nextCursor, nil
}

func (s *Service) enrichUsers(ctx context.Context, docs []opensearch.UserDoc, viewerID string) []UserResult {
	if len(docs) == 0 {
		return []UserResult{}
	}
	ids := make([]string, len(docs))
	for i, d := range docs {
		ids[i] = d.ID
	}

	counts, err := s.users.BatchGetFollowerCounts(ctx, ids)
	if err != nil {
		s.log.ErrorContext(ctx, "search users: batch follower counts — falling back to zero", "err", err)
		counts = map[string]int64{}
	}

	var followState map[string]bool
	if viewerID != "" {
		followState, err = s.users.GetFollowState(ctx, viewerID, ids)
		if err != nil {
			s.log.ErrorContext(ctx, "search users: get follow state — falling back to false", "err", err)
			followState = map[string]bool{}
		}
	}

	out := make([]UserResult, len(docs))
	for i, d := range docs {
		out[i] = UserResult{
			Doc:           d,
			FollowerCount: counts[d.ID],
			IsFollowing:   followState[d.ID],
		}
	}
	return out
}

func normaliseMode(mode string) string {
	switch mode {
	case ModeKeyword, ModeSemantic:
		return mode
	default:
		return ModeHybrid
	}
}

func clampLimit(n int) int {
	if n <= 0 || n > maxLimit {
		return defaultLimit
	}
	return n
}

func decodeCursor(cursor *string) int {
	if cursor == nil {
		return 0
	}
	n, err := strconv.Atoi(*cursor)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func nextCursorFor(from, limit, got int) *string {
	if got < limit {
		return nil
	}
	next := strconv.Itoa(from + limit)
	return &next
}
