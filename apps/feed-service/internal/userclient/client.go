// Package userclient calls user-service internal endpoints via gRPC.
// Used by the fan-out consumer to fetch follower IDs without a DB dependency.
package userclient

import (
	"context"

	"github.com/sony/gobreaker"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/twitter/shared/breaker"
	"github.com/twitter/shared/grpcretry"
	userv1 "github.com/twitter/shared/proto/gen/user/v1"
)

type Client struct {
	conn         *grpc.ClientConn
	stub         userv1.UserInternalClient
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
		grpc.WithUnaryInterceptor(grpcretry.UnaryClientInterceptor(grpcretry.Options{})),
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

func (c *Client) GetFollowerIDs(ctx context.Context, userID string) ([]string, error) {
	raw, err := c.cb.Execute(func() (interface{}, error) {
		return c.stub.GetFollowerIDs(c.auth(ctx), &userv1.GetFollowerIDsRequest{UserId: userID})
	})
	if err != nil {
		return nil, err
	}
	return raw.(*userv1.GetFollowerIDsResponse).FollowerIds, nil
}

func (c *Client) GetFollowerCount(ctx context.Context, userID string) (int64, error) {
	raw, err := c.cb.Execute(func() (interface{}, error) {
		return c.stub.GetFollowerCount(c.auth(ctx), &userv1.GetFollowerCountRequest{UserId: userID})
	})
	if err != nil {
		return 0, err
	}
	return raw.(*userv1.GetFollowerCountResponse).FollowerCount, nil
}

func (c *Client) GetFollowingIDs(ctx context.Context, userID string) ([]string, error) {
	raw, err := c.cb.Execute(func() (interface{}, error) {
		return c.stub.GetFollowingIDs(c.auth(ctx), &userv1.GetFollowingIDsRequest{UserId: userID})
	})
	if err != nil {
		return nil, err
	}
	return raw.(*userv1.GetFollowingIDsResponse).FollowingIds, nil
}

func (c *Client) auth(ctx context.Context) context.Context {
	return metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+c.serviceToken)
}
