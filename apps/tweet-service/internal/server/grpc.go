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
	tweetv1 "github.com/twitter/shared/proto/gen/tweet/v1"
	db "github.com/twitter/tweet-service/db/sqlc"
	"github.com/twitter/tweet-service/internal/hashtag"
)

type grpcServer struct {
	tweetv1.UnimplementedTweetInternalServer
	s *Server
}

func (g *grpcServer) GetTweet(ctx context.Context, req *tweetv1.GetTweetRequest) (*tweetv1.GetTweetResponse, error) {
	t, err := g.s.tweet.GetByID(ctx, req.TweetId, req.ViewerId)
	if err != nil {
		if errors.Is(err, httperr.ErrNotFound) {
			return &tweetv1.GetTweetResponse{Found: false}, nil
		}
		return nil, status.Errorf(codes.Internal, "get tweet: %v", err)
	}
	return &tweetv1.GetTweetResponse{Found: true, Tweet: toProtoTweet(t)}, nil
}

func (g *grpcServer) GetTweetsByAuthor(ctx context.Context, req *tweetv1.GetTweetsByAuthorRequest) (*tweetv1.GetTweetsByAuthorResponse, error) {
	limit := req.Limit
	if limit <= 0 || limit > 100 {
		limit = 20
	}
	tweets, err := g.s.tweet.GetByAuthor(ctx, req.AuthorId, "", limit, nil)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get tweets by author: %v", err)
	}
	protoTweets := make([]*tweetv1.Tweet, len(tweets))
	for i, t := range tweets {
		protoTweets[i] = toProtoTweet(db.GetTweetByIDRow{
			ID:           t.ID,
			AuthorID:     t.AuthorID,
			Body:         t.Body,
			LikeCount:    t.LikeCount,
			RetweetCount: t.RetweetCount,
			ReplyCount:   t.ReplyCount,
			ReplyToID:    t.ReplyToID,
			MediaID:      t.MediaID,
			MediaUrl:     t.MediaUrl,
			CreatedAt:    t.CreatedAt,
		})
	}
	return &tweetv1.GetTweetsByAuthorResponse{Tweets: protoTweets}, nil
}

func (g *grpcServer) GetRecentPopularTweets(ctx context.Context, req *tweetv1.GetRecentPopularTweetsRequest) (*tweetv1.GetRecentPopularTweetsResponse, error) {
	limit := req.Limit
	if limit <= 0 || limit > 200 {
		limit = 100
	}
	rows, err := g.s.tweet.GetRecentPopular(ctx, limit)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get recent popular: %v", err)
	}
	tweets := make([]*tweetv1.Tweet, len(rows))
	for i, r := range rows {
		tweets[i] = &tweetv1.Tweet{
			Id:           r.ID,
			AuthorId:     r.AuthorID,
			Body:         r.Body,
			LikeCount:    r.LikeCount,
			RetweetCount: r.RetweetCount,
			ReplyCount:   r.ReplyCount,
			Hashtags:     hashtag.Extract(r.Body),
			CreatedAt:    r.CreatedAt.Unix(),
		}
	}
	return &tweetv1.GetRecentPopularTweetsResponse{Tweets: tweets}, nil
}

func (g *grpcServer) GetInteractions(ctx context.Context, req *tweetv1.GetInteractionsRequest) (*tweetv1.GetInteractionsResponse, error) {
	if len(req.TweetIds) == 0 || req.ViewerId == "" {
		return &tweetv1.GetInteractionsResponse{Interactions: map[string]*tweetv1.TweetInteraction{}}, nil
	}
	rows, err := g.s.tweet.GetInteractionsByIDs(ctx, req.TweetIds, req.ViewerId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "get interactions: %v", err)
	}
	interactions := make(map[string]*tweetv1.TweetInteraction, len(rows))
	for _, r := range rows {
		interactions[r.TweetID] = &tweetv1.TweetInteraction{
			IsLiked:     r.IsLiked,
			IsRetweeted: r.IsRetweeted,
		}
	}
	return &tweetv1.GetInteractionsResponse{Interactions: interactions}, nil
}

func toProtoTweet(t db.GetTweetByIDRow) *tweetv1.Tweet {
	pt := &tweetv1.Tweet{
		Id:           t.ID,
		AuthorId:     t.AuthorID,
		Body:         t.Body,
		LikeCount:    t.LikeCount,
		RetweetCount: t.RetweetCount,
		ReplyCount:   t.ReplyCount,
		Hashtags:     hashtag.Extract(t.Body),
		CreatedAt:    t.CreatedAt.Unix(),
		IsLiked:      t.IsLiked,
		IsRetweeted:  t.IsRetweeted,
	}
	if t.ReplyToID != nil {
		pt.ReplyToId = *t.ReplyToID
	}
	if t.MediaID != nil {
		pt.MediaId = *t.MediaID
	}
	return pt
}

func (s *Server) startGRPC(addr string) *grpc.Server {
	gs := grpc.NewServer(
		grpc.StatsHandler(otelgrpc.NewServerHandler()),
		grpc.ChainUnaryInterceptor(
			metrics.GRPCServerInterceptor("tweet-service"),
			serviceTokenInterceptor(s.serviceToken),
		),
	)
	tweetv1.RegisterTweetInternalServer(gs, &grpcServer{s: s})

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
