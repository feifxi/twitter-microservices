package userclient

import (
	"context"
	"errors"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"
	"github.com/sony/gobreaker"
	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/twitter/shared/breaker"
	"github.com/twitter/shared/grpcretry"
	userv1 "github.com/twitter/shared/proto/gen/user/v1"
)

const userCountsKey = "user:counts:%s"

type Client struct {
	conn         *grpc.ClientConn
	stub         userv1.UserInternalClient
	rdb          *redis.Client
	serviceToken string
	cb           *gobreaker.CircuitBreaker
}

func New(target, serviceToken string, rdb *redis.Client) (*Client, error) {
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
		rdb:          rdb,
		serviceToken: serviceToken,
		cb:           breaker.New("user-service"),
	}, nil
}

func (c *Client) Close() error { return c.conn.Close() }

// BatchGetFollowerCounts reads from user:counts:{id} (written by user-service
// on every follow/unfollow). Missing keys yield 0 for that user — caller
// already treats absent counts as zero.
func (c *Client) BatchGetFollowerCounts(ctx context.Context, userIDs []string) (map[string]int64, error) {
	if len(userIDs) == 0 {
		return map[string]int64{}, nil
	}
	pipe := c.rdb.Pipeline()
	cmds := make([]*redis.StringCmd, len(userIDs))
	for i, id := range userIDs {
		cmds[i] = pipe.HGet(ctx, fmt.Sprintf(userCountsKey, id), "follower_count")
	}
	if _, err := pipe.Exec(ctx); err != nil && !errors.Is(err, redis.Nil) {
		return nil, err
	}
	out := make(map[string]int64, len(userIDs))
	for i, cmd := range cmds {
		v, err := cmd.Result()
		if err != nil {
			continue
		}
		n, _ := strconv.ParseInt(v, 10, 64)
		out[userIDs[i]] = n
	}
	return out, nil
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
