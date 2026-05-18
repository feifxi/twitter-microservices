package userclient

import (
	"context"

	"github.com/sony/gobreaker"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/twitter/shared/breaker"
	userv1 "github.com/twitter/shared/proto/gen/user/v1"
)

type Client struct {
	conn         *grpc.ClientConn
	stub         userv1.UserInternalClient
	serviceToken string
	cb           *gobreaker.CircuitBreaker
}

func New(target, serviceToken string) (*Client, error) {
	conn, err := grpc.NewClient(target,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(otelgrpc.NewClientHandler()),
	)
	if err != nil {
		return nil, err
	}
	return &Client{
		conn:         conn,
		stub:         userv1.NewUserInternalClient(conn),
		serviceToken: serviceToken,
		cb:           breaker.New("user-service"),
	}, nil
}

func (c *Client) Close() error { return c.conn.Close() }

func (c *Client) BatchGetFollowerCounts(ctx context.Context, userIDs []string) (map[string]int64, error) {
	if len(userIDs) == 0 {
		return map[string]int64{}, nil
	}
	raw, err := c.cb.Execute(func() (interface{}, error) {
		return c.stub.BatchGetFollowerCounts(c.auth(ctx), &userv1.BatchGetFollowerCountsRequest{UserIds: userIDs})
	})
	if err != nil {
		return nil, err
	}
	return raw.(*userv1.BatchGetFollowerCountsResponse).FollowerCounts, nil
}

func (c *Client) GetFollowState(ctx context.Context, viewerID string, targetIDs []string) (map[string]bool, error) {
	if viewerID == "" || len(targetIDs) == 0 {
		return map[string]bool{}, nil
	}
	raw, err := c.cb.Execute(func() (interface{}, error) {
		return c.stub.GetFollowState(c.auth(ctx), &userv1.GetFollowStateRequest{ViewerId: viewerID, TargetIds: targetIDs})
	})
	if err != nil {
		return nil, err
	}
	return raw.(*userv1.GetFollowStateResponse).IsFollowing, nil
}

func (c *Client) auth(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+c.serviceToken)
}
