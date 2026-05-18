package indexer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/twitter/shared/dlq"
	"github.com/twitter/shared/events"
	"github.com/twitter/shared/metrics"
	"github.com/twitter/shared/tracing"
	"github.com/twitter/search-service/internal/embeddings"
	"github.com/twitter/search-service/internal/opensearch"
)

// poison messages route to DLQ and commit through to avoid stalling the partition
var errUnmarshal = errors.New("indexer: malformed event payload")

type Indexer struct {
	os    *opensearch.Client
	embed *embeddings.Client
	dlq   *kafka.Writer
	log   *slog.Logger
}

func New(os *opensearch.Client, embed *embeddings.Client, log *slog.Logger) *Indexer {
	return &Indexer{os: os, embed: embed, log: log}
}

func (idx *Indexer) WithDLQ(dlq *kafka.Writer) *Indexer {
	idx.dlq = dlq
	return idx
}

// At-least-once: transient errors leave offset uncommitted for redelivery; poison messages commit through.
func (idx *Indexer) Run(ctx context.Context, r *kafka.Reader) {
	for {
		msg, err := r.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			idx.log.ErrorContext(ctx, "kafka fetch", "group", r.Config().GroupID, "err", err)
			continue
		}
		if err := idx.dispatch(ctx, msg); err != nil {
			metrics.KafkaConsumerErrors.WithLabelValues("search-service", msg.Topic).Inc()
			if errors.Is(err, errUnmarshal) {
				dlq.Write(ctx, idx.dlq, idx.log, msg, err.Error())
				idx.log.ErrorContext(ctx, "poison message; routed to DLQ", "topic", msg.Topic, "offset", msg.Offset, "err", err)
			} else {
				idx.log.ErrorContext(ctx, "dispatch failed; will retry", "topic", msg.Topic, "offset", msg.Offset, "err", err)
				continue
			}
		}
		if err := r.CommitMessages(ctx, msg); err != nil {
			idx.log.ErrorContext(ctx, "kafka commit", "topic", msg.Topic, "offset", msg.Offset, "err", err)
		}
	}
}

func (idx *Indexer) dispatch(ctx context.Context, msg kafka.Message) error {
	ctx = tracing.FromKafkaMessage(ctx, msg)
	switch msg.Topic {
	case events.TopicTweetCreated:
		var evt events.TweetCreatedEvent
		if err := json.Unmarshal(msg.Value, &evt); err != nil {
			return fmt.Errorf("%w: tweet.created: %v", errUnmarshal, err)
		}
		return idx.indexTweet(ctx, evt)

	case events.TopicTweetDeleted:
		var evt events.TweetDeletedEvent
		if err := json.Unmarshal(msg.Value, &evt); err != nil {
			return fmt.Errorf("%w: tweet.deleted: %v", errUnmarshal, err)
		}
		return idx.os.DeleteTweet(ctx, evt.TweetID)

	case events.TopicUserCreated:
		var evt events.UserCreatedEvent
		if err := json.Unmarshal(msg.Value, &evt); err != nil {
			return fmt.Errorf("%w: user.created: %v", errUnmarshal, err)
		}
		return idx.indexUserCreated(ctx, evt)

	case events.TopicUserUpdated:
		var evt events.UserUpdatedEvent
		if err := json.Unmarshal(msg.Value, &evt); err != nil {
			return fmt.Errorf("%w: user.updated: %v", errUnmarshal, err)
		}
		return idx.indexUserUpdated(ctx, evt)
	}
	return nil
}

func (idx *Indexer) indexTweet(ctx context.Context, evt events.TweetCreatedEvent) error {
	doc := opensearch.TweetDoc{
		ID:        evt.TweetID,
		AuthorID:  evt.AuthorID,
		Body:      evt.Body,
		Hashtags:  evt.Hashtags,
		ReplyToID: evt.ReplyToID,
		MediaID:   evt.MediaID,
		MediaURL:  evt.MediaURL,
		CreatedAt: evt.CreatedAt,
	}

	vec, err := idx.embed.Embed(ctx, evt.Body)
	if err != nil {
		// Tolerated: index without vector so keyword search still works
		idx.log.ErrorContext(ctx, "embed tweet — indexing without vector", "tweet_id", evt.TweetID, "err", err)
	} else {
		doc.Vector = vec
	}

	return idx.os.IndexTweet(ctx, doc)
}

func (idx *Indexer) indexUserCreated(ctx context.Context, evt events.UserCreatedEvent) error {
	doc := opensearch.UserDoc{
		ID:          evt.UserID,
		Username:    evt.Username,
		DisplayName: evt.DisplayName,
		Bio:         evt.Bio,
		AvatarURL:   evt.AvatarURL,
		UpdatedAt:   evt.CreatedAt,
	}

	text := evt.Username
	if evt.Bio != "" {
		text = evt.Username + " " + evt.Bio
	}
	vec, err := idx.embed.Embed(ctx, text)
	if err != nil {
		idx.log.ErrorContext(ctx, "embed user — indexing without vector", "user_id", evt.UserID, "err", err)
	} else {
		doc.Vector = vec
	}

	return idx.os.IndexUser(ctx, doc)
}

func (idx *Indexer) indexUserUpdated(ctx context.Context, evt events.UserUpdatedEvent) error {
	text := evt.Username
	if evt.Bio != "" {
		text = evt.Username + " " + evt.Bio
	}

	// IndexUser is an upsert that REPLACES the doc — every field must be written
	doc := opensearch.UserDoc{
		ID:          evt.UserID,
		Username:    evt.Username,
		DisplayName: evt.DisplayName,
		Bio:         evt.Bio,
		AvatarURL:   evt.AvatarURL,
		UpdatedAt:   time.Now(),
	}

	vec, err := idx.embed.Embed(ctx, text)
	if err != nil {
		idx.log.ErrorContext(ctx, "embed user update — indexing without vector", "user_id", evt.UserID, "err", err)
	} else {
		doc.Vector = vec
	}

	return idx.os.IndexUser(ctx, doc)
}
