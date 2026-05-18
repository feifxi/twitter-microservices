package tweetclient

import (
	"context"
	"time"

	"github.com/sony/gobreaker"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/twitter/shared/breaker"
	tweetv1 "github.com/twitter/shared/proto/gen/tweet/v1"
)

type Author struct {
	ID          string  `json:"id"`
	Username    string  `json:"username"`
	DisplayName string  `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
}

// Always an original — retweets are conveyed via the feed-item wrapper.
type Tweet struct {
	ID           string    `json:"id"`
	Type         string    `json:"type"`
	Author       *Author   `json:"author"`
	AuthorID     string    `json:"author_id"`
	Body         string    `json:"body"`
	ReplyToID    *string   `json:"reply_to_id"`
	MediaID      *string   `json:"media_id"`
	MediaURL     *string   `json:"media_url"`
	LikeCount    int32     `json:"like_count"`
	RetweetCount int32     `json:"retweet_count"`
	ReplyCount   int32     `json:"reply_count"`
	Hashtags     []string  `json:"hashtags"`
	IsLiked      bool      `json:"is_liked"`
	IsRetweeted  bool      `json:"is_retweeted"`
	CreatedAt    time.Time `json:"created_at"`
}

// Counts are NOT included here — they come from Redis tweet:counts:{id}.
type TweetInteraction struct {
	IsLiked     bool
	IsRetweeted bool
}

type Client struct {
	conn         *grpc.ClientConn
	stub         tweetv1.TweetInternalClient
	serviceToken string
	cb           *gobreaker.CircuitBreaker
}

func New(target, serviceToken string) (*Client, error) {
	// Plain gRPC is fine here: in production, mTLS is expected to be handled
	// transparently by a service mesh / sidecar (Istio, Linkerd, App Mesh).
	// Only swap for credentials.NewTLS(...) if this client ever talks to a
	// gRPC server that's exposed without a mesh in front of it.
	conn, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return nil, err
	}
	return &Client{
		conn:         conn,
		stub:         tweetv1.NewTweetInternalClient(conn),
		serviceToken: serviceToken,
		cb:           breaker.New("tweet-service"),
	}, nil
}

func (c *Client) Close() error { return c.conn.Close() }

func (c *Client) GetInteractions(ctx context.Context, tweetIDs []string, viewerID string) (map[string]TweetInteraction, error) {
	raw, err := c.cb.Execute(func() (interface{}, error) {
		return c.stub.GetInteractions(c.auth(ctx), &tweetv1.GetInteractionsRequest{
			TweetIds: tweetIDs,
			ViewerId: viewerID,
		})
	})
	if err != nil {
		return nil, err
	}
	resp := raw.(*tweetv1.GetInteractionsResponse)
	result := make(map[string]TweetInteraction, len(resp.Interactions))
	for id, s := range resp.Interactions {
		result[id] = TweetInteraction{
			IsLiked:     s.IsLiked,
			IsRetweeted: s.IsRetweeted,
		}
	}
	return result, nil
}

func (c *Client) GetRecentTweets(ctx context.Context, authorID string, limit int) ([]*Tweet, error) {
	raw, err := c.cb.Execute(func() (interface{}, error) {
		return c.stub.GetTweetsByAuthor(c.auth(ctx), &tweetv1.GetTweetsByAuthorRequest{
			AuthorId: authorID,
			Limit:    int32(limit),
		})
	})
	if err != nil {
		return nil, err
	}
	resp := raw.(*tweetv1.GetTweetsByAuthorResponse)
	tweets := make([]*Tweet, len(resp.Tweets))
	for i, t := range resp.Tweets {
		tweets[i] = fromProto(t)
	}
	return tweets, nil
}

func (c *Client) GetRecentPopularTweets(ctx context.Context, limit int) ([]*Tweet, error) {
	raw, err := c.cb.Execute(func() (interface{}, error) {
		return c.stub.GetRecentPopularTweets(c.auth(ctx), &tweetv1.GetRecentPopularTweetsRequest{Limit: int32(limit)})
	})
	if err != nil {
		return nil, err
	}
	resp := raw.(*tweetv1.GetRecentPopularTweetsResponse)
	tweets := make([]*Tweet, len(resp.Tweets))
	for i, t := range resp.Tweets {
		tweets[i] = fromProto(t)
	}
	return tweets, nil
}

func (c *Client) auth(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+c.serviceToken)
}

func fromProto(t *tweetv1.Tweet) *Tweet {
	tw := &Tweet{
		ID:           t.Id,
		AuthorID:     t.AuthorId,
		Body:         t.Body,
		LikeCount:    t.LikeCount,
		RetweetCount: t.RetweetCount,
		ReplyCount:   t.ReplyCount,
		Hashtags:     t.Hashtags,
		IsLiked:      t.IsLiked,
		IsRetweeted:  t.IsRetweeted,
		CreatedAt:    time.Unix(t.CreatedAt, 0).UTC(),
	}
	switch {
	case t.ReplyToId != "":
		tw.Type = "reply"
		s := t.ReplyToId
		tw.ReplyToID = &s
	default:
		tw.Type = "tweet"
	}
	if t.MediaId != "" {
		s := t.MediaId
		tw.MediaID = &s
	}
	return tw
}
