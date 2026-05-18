package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"

	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"github.com/twitter/shared/dlq"
	"github.com/twitter/shared/events"
	"github.com/twitter/shared/metrics"
	"github.com/twitter/shared/outbox"
	"github.com/twitter/shared/tracing"
)

// eventID == 0 disables dedup (tests without the header).
type EventHandler interface {
	CreateFromLike(ctx context.Context, eventID int64, actorID, authorID, tweetID string) error
	CreateFromRetweet(ctx context.Context, eventID int64, retweeterID, originalAuthorID, tweetID string) error
	DeleteFromRetweet(ctx context.Context, retweeterID, tweetID string) error
	CreateFromReply(ctx context.Context, eventID int64, replierID, parentAuthorID, parentTweetID string) error
	CreateFromFollow(ctx context.Context, eventID int64, followerID, followeeID string) error
}

// Best-effort pre-check; correctness relies on EventHandler recording dedup atomically with side effects.
type DedupStore interface {
	IsEventProcessed(ctx context.Context, eventID int64) (bool, error)
}

// Poison-message sentinel: routed to DLQ then committed through so the partition keeps moving.
var errUnmarshal = errors.New("consumer: malformed event payload")

type Consumer struct {
	svc   EventHandler
	dedup DedupStore
	rdb   *redis.Client
	dlq   *kafka.Writer
	log   *slog.Logger
}

func New(svc EventHandler, dedup DedupStore, rdb *redis.Client, dlq *kafka.Writer, log *slog.Logger) *Consumer {
	return &Consumer{svc: svc, dedup: dedup, rdb: rdb, dlq: dlq, log: log}
}

// At-least-once: offset committed only after dispatch succeeds; poison messages routed to DLQ then committed through.
func (c *Consumer) Run(ctx context.Context, r *kafka.Reader) {
	for {
		msg, err := r.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			c.log.ErrorContext(ctx, "kafka fetch", "group", r.Config().GroupID, "err", err)
			continue
		}

		if skip, err := c.isDuplicate(ctx, msg); err != nil {
			c.log.ErrorContext(ctx, "dedup check failed; will retry", "topic", msg.Topic, "offset", msg.Offset, "err", err)
			continue
		} else if skip {
			if err := r.CommitMessages(ctx, msg); err != nil {
				c.log.ErrorContext(ctx, "kafka commit (dup)", "topic", msg.Topic, "offset", msg.Offset, "err", err)
			}
			continue
		}

		if err := c.dispatch(ctx, extractEventID(msg), msg); err != nil {
			metrics.KafkaConsumerErrors.WithLabelValues("notification-service", msg.Topic).Inc()
			if errors.Is(err, errUnmarshal) {
				dlq.Write(ctx, c.dlq, c.log, msg, err.Error())
				c.log.ErrorContext(ctx, "poison message; routed to DLQ", "topic", msg.Topic, "offset", msg.Offset, "err", err)
			} else {
				c.log.ErrorContext(ctx, "dispatch failed; will retry", "topic", msg.Topic, "offset", msg.Offset, "err", err)
				continue
			}
		}

		if err := r.CommitMessages(ctx, msg); err != nil {
			c.log.ErrorContext(ctx, "kafka commit", "topic", msg.Topic, "offset", msg.Offset, "err", err)
		}
	}
}

func (c *Consumer) isDuplicate(ctx context.Context, msg kafka.Message) (bool, error) {
	if c.dedup == nil {
		return false, nil
	}
	eventID := extractEventID(msg)
	if eventID == 0 {
		return false, nil
	}
	return c.dedup.IsEventProcessed(ctx, eventID)
}

func extractEventID(msg kafka.Message) int64 {
	for _, h := range msg.Headers {
		if h.Key == outbox.EventIDHeader {
			id, _ := strconv.ParseInt(string(h.Value), 10, 64)
			return id
		}
	}
	return 0
}

func (c *Consumer) dispatch(ctx context.Context, eventID int64, msg kafka.Message) error {
	ctx = tracing.FromKafkaMessage(ctx, msg)
	switch msg.Topic {
	case events.TopicTweetLiked:
		var evt events.TweetLikedEvent
		if err := json.Unmarshal(msg.Value, &evt); err != nil {
			return fmt.Errorf("%w: tweet.liked: %v", errUnmarshal, err)
		}
		return c.svc.CreateFromLike(ctx, eventID, evt.LikerID, evt.AuthorID, evt.TweetID)

	case events.TopicTweetRetweeted:
		var evt events.TweetRetweetedEvent
		if err := json.Unmarshal(msg.Value, &evt); err != nil {
			return fmt.Errorf("%w: tweet.retweeted: %v", errUnmarshal, err)
		}
		return c.svc.CreateFromRetweet(ctx, eventID, evt.RetweeterID, evt.OriginalAuthorID, evt.OriginalTweetID)

	case events.TopicTweetUnretweeted:
		var evt events.TweetUnretweetedEvent
		if err := json.Unmarshal(msg.Value, &evt); err != nil {
			return fmt.Errorf("%w: tweet.unretweeted: %v", errUnmarshal, err)
		}
		return c.svc.DeleteFromRetweet(ctx, evt.RetweeterID, evt.OriginalTweetID)

	case events.TopicTweetReplied:
		var evt events.TweetRepliedEvent
		if err := json.Unmarshal(msg.Value, &evt); err != nil {
			return fmt.Errorf("%w: tweet.replied: %v", errUnmarshal, err)
		}
		return c.svc.CreateFromReply(ctx, eventID, evt.ReplierID, evt.ParentAuthorID, evt.ParentTweetID)

	case events.TopicUserFollowed:
		var evt events.UserFollowedEvent
		if err := json.Unmarshal(msg.Value, &evt); err != nil {
			return fmt.Errorf("%w: user.followed: %v", errUnmarshal, err)
		}
		return c.svc.CreateFromFollow(ctx, eventID, evt.FollowerID, evt.FolloweeID)
	}
	return nil
}
