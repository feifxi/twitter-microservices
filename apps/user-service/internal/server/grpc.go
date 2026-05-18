package server

import (
	"context"
	"errors"
	"net"
	"os"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/twitter/shared/httperr"
	"github.com/twitter/shared/metrics"
	userv1 "github.com/twitter/shared/proto/gen/user/v1"
	"github.com/twitter/shared/ptr"
)

type grpcServer struct {
	userv1.UnimplementedUserInternalServer
	s *Server
}

func (g *grpcServer) GetFollowerIDs(ctx context.Context, req *userv1.GetFollowerIDsRequest) (*userv1.GetFollowerIDsResponse, error) {
	ids, err := g.s.user.GetFollowerIDs(ctx, req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get follower ids: %v", err)
	}
	return &userv1.GetFollowerIDsResponse{FollowerIds: ids}, nil
}

func (g *grpcServer) GetFollowingIDs(ctx context.Context, req *userv1.GetFollowingIDsRequest) (*userv1.GetFollowingIDsResponse, error) {
	ids, err := g.s.user.GetFollowingIDs(ctx, req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get following ids: %v", err)
	}
	return &userv1.GetFollowingIDsResponse{FollowingIds: ids}, nil
}

func (g *grpcServer) GetFollowerCount(ctx context.Context, req *userv1.GetFollowerCountRequest) (*userv1.GetFollowerCountResponse, error) {
	count, err := g.s.user.GetFollowerCount(ctx, req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get follower count: %v", err)
	}
	return &userv1.GetFollowerCountResponse{FollowerCount: count}, nil
}

func (g *grpcServer) BatchGetFollowerCounts(ctx context.Context, req *userv1.BatchGetFollowerCountsRequest) (*userv1.BatchGetFollowerCountsResponse, error) {
	counts, err := g.s.user.BatchGetFollowerCounts(ctx, req.UserIds)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "batch get follower counts: %v", err)
	}
	return &userv1.BatchGetFollowerCountsResponse{FollowerCounts: counts}, nil
}

func (g *grpcServer) GetFollowState(ctx context.Context, req *userv1.GetFollowStateRequest) (*userv1.GetFollowStateResponse, error) {
	state, err := g.s.user.GetFollowState(ctx, req.ViewerId, req.TargetIds)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get follow state: %v", err)
	}
	return &userv1.GetFollowStateResponse{IsFollowing: state}, nil
}

func (g *grpcServer) GetUserByID(ctx context.Context, req *userv1.GetUserByIDRequest) (*userv1.GetUserByIDResponse, error) {
	u, err := g.s.user.GetUserByID(ctx, req.UserId)
	if err != nil {
		if errors.Is(err, httperr.ErrNotFound) {
			return nil, status.Error(codes.NotFound, "user not found")
		}
		return nil, status.Errorf(codes.Internal, "get user: %v", err)
	}
	return &userv1.GetUserByIDResponse{
		UserId:      u.ID,
		Username:    ptr.Deref(u.Username),
		DisplayName: ptr.Deref(u.DisplayName),
		AvatarUrl:   ptr.Deref(u.AvatarUrl),
	}, nil
}

func (s *Server) startGRPC(addr string) *grpc.Server {
	gs := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(
			metrics.GRPCServerInterceptor("user-service"),
			serviceTokenInterceptor(s.serviceToken),
		),
	)
	userv1.RegisterUserInternalServer(gs, &grpcServer{s: s})

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		s.log.Error("grpc listen", "addr", addr, "err", err)
		os.Exit(1)
	}
	go func() {
		s.log.Info("grpc server listening", "addr", addr)
		if err := gs.Serve(lis); err != nil {
			s.log.Error("grpc serve", "err", err)
		}
	}()
	return gs
}

func serviceTokenInterceptor(token string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}
		vals := md.Get("authorization")
		if len(vals) == 0 || vals[0] != "Bearer "+token {
			return nil, status.Error(codes.Unauthenticated, "invalid service token")
		}
		return handler(ctx, req)
	}
}
