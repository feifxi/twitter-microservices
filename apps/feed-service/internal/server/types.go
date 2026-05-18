package server

import (
	"time"

	"github.com/twitter/feed-service/internal/consumer"
	"github.com/twitter/feed-service/internal/feed"
	"github.com/twitter/feed-service/internal/tweetclient"
)

// Wire shape mirrors tweet-service so a tweet rendered from /v1/feed/* is
// byte-identical to one from /v1/users/:id/tweets.

type authorResponse struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
}

// Retweets ride in feedItem with kind="retweet", not here.
type tweetResponse struct {
	ID           string         `json:"id"`
	Type         string         `json:"type"` // "tweet" | "reply"
	Author       authorResponse `json:"author"`
	Body         string         `json:"body"`
	ReplyToID    *string        `json:"reply_to_id"`
	MediaID      *string        `json:"media_id"`
	MediaURL     *string        `json:"media_url"`
	LikeCount    int32          `json:"like_count"`
	RetweetCount int32          `json:"retweet_count"`
	ReplyCount   int32          `json:"reply_count"`
	Hashtags     []string       `json:"hashtags"`
	IsLiked      bool           `json:"is_liked"`
	IsRetweeted  bool           `json:"is_retweeted"`
	CreatedAt    time.Time      `json:"created_at"`
}

// Original tweet rides separately in feedItem.Tweet.
type retweetWrapper struct {
	ID        string         `json:"id"`
	Retweeter authorResponse `json:"retweeter"`
	CreatedAt time.Time      `json:"created_at"`
}

// Tweet is a value type (not pointer) so it cannot serialize as JSON null; matches tweet-service's wire shape.
type feedItem struct {
	Kind    string          `json:"kind"`
	Tweet   tweetResponse   `json:"tweet"`
	Retweet *retweetWrapper `json:"retweet"`
}

type feedResponse struct {
	Items      []feedItem `json:"items"`
	NextCursor *string    `json:"next_cursor"`
}

type trendingResponse struct {
	Trending []consumer.TrendingTag `json:"trending"`
}

// Always returns a non-nil slice so JSON renders `[]`, never null.
func newFeedItems(items []feed.FeedItem) []feedItem {
	out := make([]feedItem, 0, len(items))
	for _, it := range items {
		if it.Tweet == nil {
			// Defensive — feed.Service should never produce an item without
			// the underlying tweet, but skip rather than panic on the wire.
			continue
		}
		out = append(out, newFeedItem(it))
	}
	return out
}

func newFeedItem(it feed.FeedItem) feedItem {
	wire := feedItem{
		Kind:  it.Kind,
		Tweet: newTweetResponse(it.Tweet),
	}
	if it.Kind == "retweet" && it.Retweet != nil {
		wire.Retweet = &retweetWrapper{
			ID:        it.Retweet.ID,
			Retweeter: newAuthorResponse(it.Retweet.Retweeter),
			CreatedAt: it.Retweet.CreatedAt,
		}
	}
	return wire
}

func newTweetResponse(t *tweetclient.Tweet) tweetResponse {
	hashtags := t.Hashtags
	if hashtags == nil {
		hashtags = []string{}
	}
	return tweetResponse{
		ID:           t.ID,
		Type:         t.Type,
		Author:       newAuthorResponse(t.Author),
		Body:         t.Body,
		ReplyToID:    t.ReplyToID,
		MediaID:      t.MediaID,
		MediaURL:     t.MediaURL,
		LikeCount:    t.LikeCount,
		RetweetCount: t.RetweetCount,
		ReplyCount:   t.ReplyCount,
		Hashtags:     hashtags,
		IsLiked:      t.IsLiked,
		IsRetweeted:  t.IsRetweeted,
		CreatedAt:    t.CreatedAt,
	}
}

// A nil Author renders as an object with zero ID and null profile fields — the FE handles missing snapshots.
func newAuthorResponse(a *tweetclient.Author) authorResponse {
	if a == nil {
		return authorResponse{}
	}
	return authorResponse{
		ID:          a.ID,
		Username:    a.Username,
		DisplayName: a.DisplayName,
		AvatarURL:   a.AvatarURL,
	}
}
