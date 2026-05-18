package tweetclient

import (
	"context"

	"github.com/sony/gobreaker"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/twitter/shared/breaker"
	"github.com/twitter/shared/grpcretry"
	tweetv1 "github.com/twitter/shared/proto/gen/tweet/v1"
)

type Interaction struct {
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
	conn, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
		grpc.WithUnaryInterceptor(grpcretry.UnaryClientInterceptor(grpcretry.Options{})),
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

func (c *Client) GetInteractions(ctx context.Context, viewerID string, tweetIDs []string) (map[string]Interaction, error) {
	if viewerID == "" || len(tweetIDs) == 0 {
		return map[string]Interaction{}, nil
	}
	raw, err := c.cb.Execute(func() (interface{}, error) {
		return c.stub.GetInteractions(c.auth(ctx), &tweetv1.GetInteractionsRequest{
			ViewerId: viewerID,
			TweetIds: tweetIDs,
		})
	})
	if err != nil {
		return nil, err
	}
	resp := raw.(*tweetv1.GetInteractionsResponse)
	out := make(map[string]Interaction, len(resp.Interactions))
	for id, s := range resp.Interactions {
		out[id] = Interaction{IsLiked: s.IsLiked, IsRetweeted: s.IsRetweeted}
	}
	return out, nil
}

func (c *Client) auth(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+c.serviceToken)
}
