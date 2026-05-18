package enrich

import (
	"context"
	"fmt"
	"strconv"

	"github.com/redis/go-redis/v9"

	"github.com/twitter/search-service/internal/search"
	"github.com/twitter/search-service/internal/tweetclient"
)

const (
	userSnapshotKey = "user:snapshot:%s"
	tweetCountsKey  = "tweet:counts:%s"
)

type TweetClient interface {
	GetInteractions(ctx context.Context, viewerID string, tweetIDs []string) (map[string]tweetclient.Interaction, error)
}

type Enricher struct {
	rdb    *redis.Client
	tweets TweetClient
}

func New(rdb *redis.Client, tweets TweetClient) *Enricher {
	return &Enricher{rdb: rdb, tweets: tweets}
}

var _ search.TweetEnricher = (*Enricher)(nil)

func (e *Enricher) BatchAuthorSnapshots(ctx context.Context, authorIDs []string) map[string]search.AuthorSnapshot {
	if len(authorIDs) == 0 {
		return map[string]search.AuthorSnapshot{}
	}
	type entry struct {
		id  string
		cmd *redis.MapStringStringCmd
	}
	cmds := make([]entry, 0, len(authorIDs))
	pipe := e.rdb.Pipeline()
	for _, id := range authorIDs {
		cmds = append(cmds, entry{id: id, cmd: pipe.HGetAll(ctx, fmt.Sprintf(userSnapshotKey, id))})
	}
	_, _ = pipe.Exec(ctx)
	out := make(map[string]search.AuthorSnapshot, len(cmds))
	for _, c := range cmds {
		fields, err := c.cmd.Result()
		if err != nil || len(fields) == 0 {
			continue
		}
		out[c.id] = search.AuthorSnapshot{
			Username:    fields["username"],
			DisplayName: fields["display_name"],
			AvatarURL:   fields["avatar_url"],
		}
	}
	return out
}

func (e *Enricher) BatchTweetCounts(ctx context.Context, tweetIDs []string) map[string]search.TweetCounts {
	if len(tweetIDs) == 0 {
		return map[string]search.TweetCounts{}
	}
	type entry struct {
		id  string
		cmd *redis.MapStringStringCmd
	}
	cmds := make([]entry, 0, len(tweetIDs))
	pipe := e.rdb.Pipeline()
	for _, id := range tweetIDs {
		cmds = append(cmds, entry{id: id, cmd: pipe.HGetAll(ctx, fmt.Sprintf(tweetCountsKey, id))})
	}
	_, _ = pipe.Exec(ctx)
	out := make(map[string]search.TweetCounts, len(cmds))
	for _, c := range cmds {
		fields, err := c.cmd.Result()
		if err != nil || len(fields) == 0 {
			continue
		}
		out[c.id] = search.TweetCounts{
			LikeCount:    parseInt32(fields["like_count"]),
			RetweetCount: parseInt32(fields["retweet_count"]),
			ReplyCount:   parseInt32(fields["reply_count"]),
		}
	}
	return out
}

func (e *Enricher) GetInteractions(ctx context.Context, viewerID string, tweetIDs []string) (map[string]search.TweetInteraction, error) {
	raw, err := e.tweets.GetInteractions(ctx, viewerID, tweetIDs)
	if err != nil {
		return nil, err
	}
	out := make(map[string]search.TweetInteraction, len(raw))
	for id, i := range raw {
		out[id] = search.TweetInteraction{IsLiked: i.IsLiked, IsRetweeted: i.IsRetweeted}
	}
	return out, nil
}

func parseInt32(s string) int32 {
	if s == "" {
		return 0
	}
	n, err := strconv.ParseInt(s, 10, 32)
	if err != nil {
		return 0
	}
	return int32(n)
}
