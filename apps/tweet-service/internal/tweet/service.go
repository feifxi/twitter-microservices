package tweet

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/oklog/ulid/v2"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"github.com/twitter/shared/events"
	"github.com/twitter/shared/httperr"
	"github.com/twitter/shared/metrics"
	"github.com/twitter/shared/outbox"
	"github.com/twitter/shared/pgxutil"
	"github.com/twitter/shared/ptr"
	"github.com/twitter/shared/tracing"
	db "github.com/twitter/tweet-service/db/sqlc"
	"github.com/twitter/tweet-service/internal/hashtag"
)

type AuthorInfo struct {
	ID          string
	Username    string
	DisplayName string
	AvatarURL   string
}

type Service struct {
	store db.Store
	kb    *kafka.Writer
	rdb   *redis.Client
	log   *slog.Logger
}

func New(store db.Store, kb *kafka.Writer, rdb *redis.Client, log *slog.Logger) *Service {
	return &Service{store: store, kb: kb, rdb: rdb, log: log}
}

func hasContent(body string, mediaID *string) bool {
	return body != "" || (mediaID != nil && *mediaID != "")
}

func (s *Service) Create(ctx context.Context, authorID, body string, mediaID, mediaURL *string) (db.Tweet, []string, error) {
	if !hasContent(body, mediaID) {
		return db.Tweet{}, nil, httperr.New(http.StatusBadRequest, "EMPTY_TWEET", "tweet must have body or media")
	}
	tags := hashtag.Extract(body)
	id := "tw_" + ulid.Make().String()

	var tweet db.Tweet
	if err := s.store.ExecTx(ctx, func(q db.Querier) error {
		var err error
		tweet, err = q.CreateTweet(ctx, db.CreateTweetParams{
			ID:       id,
			AuthorID: authorID,
			Body:     body,
			MediaID:  mediaID,
			MediaUrl: mediaURL,
		})
		if err != nil {
			return pgxutil.MapErr(err)
		}
		return s.outboxEnqueue(ctx, q, events.TopicTweetCreated, tweet.AuthorID, events.TweetCreatedEvent{
			TweetID:   tweet.ID,
			AuthorID:  tweet.AuthorID,
			Body:      tweet.Body,
			Hashtags:  tags,
			MediaID:   ptr.Deref(mediaID),
			MediaURL:  ptr.Deref(mediaURL),
			CreatedAt: tweet.CreatedAt,
		})
	}); err != nil {
		return db.Tweet{}, nil, err
	}
	return tweet, tags, nil
}

func (s *Service) GetByID(ctx context.Context, tweetID, userID string) (db.GetTweetByIDRow, error) {
	var viewerID *string
	if userID != "" {
		viewerID = &userID
	}
	t, err := s.store.GetTweetByID(ctx, db.GetTweetByIDParams{ID: tweetID, ViewerID: viewerID})
	return t, pgxutil.MapErr(err)
}

func (s *Service) Delete(ctx context.Context, tweetID, authorID string) error {
	tweet, err := s.store.GetTweetByID(ctx, db.GetTweetByIDParams{ID: tweetID, ViewerID: nil})
	if err != nil {
		return pgxutil.MapErr(err)
	}
	if tweet.AuthorID != authorID {
		return httperr.ErrForbidden
	}

	return s.store.ExecTx(ctx, func(q db.Querier) error {
		if err := q.DeleteTweet(ctx, db.DeleteTweetParams{ID: tweetID, AuthorID: authorID}); err != nil {
			return pgxutil.MapErr(err)
		}
		return s.outboxEnqueue(ctx, q, events.TopicTweetDeleted, authorID, events.TweetDeletedEvent{
			TweetID:  tweetID,
			AuthorID: authorID,
		})
	})
}

func (s *Service) Like(ctx context.Context, userID, tweetID string) (bool, error) {
	tweet, err := s.store.GetTweetByID(ctx, db.GetTweetByIDParams{ID: tweetID, ViewerID: nil})
	if err != nil {
		return false, pgxutil.MapErr(err)
	}

	var alreadyLiked bool
	err = s.store.ExecTx(ctx, func(q db.Querier) error {
		tag, err := q.CreateLike(ctx, db.CreateLikeParams{UserID: userID, TweetID: tweetID})
		if err != nil {
			return pgxutil.MapErr(err)
		}
		if tag.RowsAffected() == 0 {
			alreadyLiked = true
			return nil
		}
		if err := q.IncrementLikeCount(ctx, tweetID); err != nil {
			s.log.WarnContext(ctx, "increment like count", "tweet_id", tweetID, "err", err)
		}
		return s.outboxEnqueue(ctx, q, events.TopicTweetLiked, tweetID, events.TweetLikedEvent{
			TweetID:  tweetID,
			LikerID:  userID,
			AuthorID: tweet.AuthorID,
			Hashtags: hashtag.Extract(tweet.Body),
		})
	})
	if err == nil && !alreadyLiked {
		s.incrCount(ctx, tweetID, "like_count", 1)
	}
	return alreadyLiked, err
}

func (s *Service) Unlike(ctx context.Context, userID, tweetID string) error {
	err := s.store.ExecTx(ctx, func(q db.Querier) error {
		if err := q.DeleteLike(ctx, db.DeleteLikeParams{UserID: userID, TweetID: tweetID}); err != nil {
			return pgxutil.MapErr(err)
		}
		if err := q.DecrementLikeCount(ctx, tweetID); err != nil {
			s.log.WarnContext(ctx, "decrement like count", "tweet_id", tweetID, "err", err)
		}
		return nil
	})
	if err == nil {
		s.incrCount(ctx, tweetID, "like_count", -1)
	}
	return err
}

// Engagement counts live on the original tweet; the retweet row carries only retweeter + when.
func (s *Service) Retweet(ctx context.Context, userID, originalID string) (db.Retweet, error) {
	orig, err := s.store.GetTweetByID(ctx, db.GetTweetByIDParams{ID: originalID, ViewerID: nil})
	if err != nil {
		return db.Retweet{}, pgxutil.MapErr(err)
	}
	retweetID := "rt_" + ulid.Make().String()

	var retweet db.Retweet
	if err := s.store.ExecTx(ctx, func(q db.Querier) error {
		var err error
		retweet, err = q.InsertRetweet(ctx, db.InsertRetweetParams{
			ID:              retweetID,
			RetweeterID:     userID,
			OriginalTweetID: originalID,
		})
		if err != nil {
			if pgxutil.IsUniqueViolation(err) {
				return httperr.New(http.StatusConflict, "ALREADY_RETWEETED", "already retweeted")
			}
			return pgxutil.MapErr(err)
		}
		if err := q.IncrementRetweetCount(ctx, originalID); err != nil {
			s.log.WarnContext(ctx, "increment retweet count", "tweet_id", originalID, "err", err)
		}
		return s.outboxEnqueue(ctx, q, events.TopicTweetRetweeted, originalID, events.TweetRetweetedEvent{
			RetweetID:        retweet.ID,
			OriginalTweetID:  originalID,
			OriginalAuthorID: orig.AuthorID,
			RetweeterID:      userID,
			CreatedAt:        retweet.CreatedAt,
		})
	}); err != nil {
		return db.Retweet{}, err
	}
	s.incrCount(ctx, originalID, "retweet_count", 1)
	return retweet, nil
}

func (s *Service) Unretweet(ctx context.Context, userID, originalID string) error {
	existing, err := s.store.GetRetweetByPair(ctx, db.GetRetweetByPairParams{
		RetweeterID:     userID,
		OriginalTweetID: originalID,
	})
	if err != nil {
		return pgxutil.MapErr(err)
	}
	if err := s.store.ExecTx(ctx, func(q db.Querier) error {
		if err := q.DeleteRetweetByPair(ctx, db.DeleteRetweetByPairParams{
			RetweeterID:     userID,
			OriginalTweetID: originalID,
		}); err != nil {
			return pgxutil.MapErr(err)
		}
		if err := q.DecrementRetweetCount(ctx, originalID); err != nil {
			s.log.WarnContext(ctx, "decrement retweet count", "tweet_id", originalID, "err", err)
		}
		return s.outboxEnqueue(ctx, q, events.TopicTweetUnretweeted, originalID, events.TweetUnretweetedEvent{
			RetweetID:       existing.ID,
			OriginalTweetID: originalID,
			RetweeterID:     userID,
		})
	}); err != nil {
		return err
	}
	s.incrCount(ctx, originalID, "retweet_count", -1)
	return nil
}

func (s *Service) Reply(ctx context.Context, authorID, parentID, body string, mediaID, mediaURL *string) (db.Tweet, error) {
	if !hasContent(body, mediaID) {
		return db.Tweet{}, httperr.New(http.StatusBadRequest, "EMPTY_TWEET", "reply must have body or media")
	}
	parent, err := s.store.GetTweetByID(ctx, db.GetTweetByIDParams{ID: parentID, ViewerID: nil})
	if err != nil {
		return db.Tweet{}, pgxutil.MapErr(err)
	}

	replyID := "tw_" + ulid.Make().String()

	var reply db.Tweet
	if err := s.store.ExecTx(ctx, func(q db.Querier) error {
		var err error
		reply, err = q.CreateReply(ctx, db.CreateReplyParams{
			ID:        replyID,
			AuthorID:  authorID,
			Body:      body,
			ReplyToID: &parentID,
			MediaID:   mediaID,
			MediaUrl:  mediaURL,
		})
		if err != nil {
			return pgxutil.MapErr(err)
		}
		if err := q.IncrementReplyCount(ctx, parentID); err != nil {
			s.log.WarnContext(ctx, "increment reply count", "tweet_id", parentID, "err", err)
		}
		return s.outboxEnqueue(ctx, q, events.TopicTweetReplied, parentID, events.TweetRepliedEvent{
			ReplyID:        replyID,
			ParentTweetID:  parentID,
			ParentAuthorID: parent.AuthorID,
			ReplierID:      authorID,
		})
	}); err != nil {
		return db.Tweet{}, err
	}
	s.incrCount(ctx, parentID, "reply_count", 1)
	return reply, nil
}

func (s *Service) GetReplies(ctx context.Context, tweetID, userID string, limit int32, cursor *string) ([]db.GetRepliesByTweetIDRow, error) {
	rows, err := s.store.GetRepliesByTweetID(ctx, db.GetRepliesByTweetIDParams{
		ReplyToID: &tweetID,
		ViewerID:  ptr.NonEmpty(userID),
		Cursor:    cursor,
		PageLimit: limit,
	})
	return rows, pgxutil.MapErr(err)
}

func (s *Service) GetRecentPopular(ctx context.Context, limit int32) ([]db.GetRecentPopularTweetsRow, error) {
	rows, err := s.store.GetRecentPopularTweets(ctx, limit)
	return rows, pgxutil.MapErr(err)
}

func (s *Service) GetByAuthor(ctx context.Context, authorID, userID string, limit int32, cursor *string) ([]db.GetTweetsByAuthorRow, error) {
	rows, err := s.store.GetTweetsByAuthor(ctx, db.GetTweetsByAuthorParams{
		AuthorID:  authorID,
		ViewerID:  ptr.NonEmpty(userID),
		Cursor:    cursor,
		PageLimit: limit,
	})
	return rows, pgxutil.MapErr(err)
}

func (s *Service) IsRetweetedByViewer(ctx context.Context, originalID, viewerID string) (bool, error) {
	if viewerID == "" {
		return false, nil
	}
	ok, err := s.store.IsRetweetedBy(ctx, db.IsRetweetedByParams{
		RetweeterID:     viewerID,
		OriginalTweetID: originalID,
	})
	return ok, pgxutil.MapErr(err)
}

func (s *Service) GetProfileTimeline(ctx context.Context, userID string, limit int32, cursor *string) ([]db.GetProfileTimelineByUserRow, error) {
	cursorAt, cursorID, err := decodeProfileCursor(cursor)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.GetProfileTimelineByUser(ctx, db.GetProfileTimelineByUserParams{
		UserID:    userID,
		PageLimit: limit,
		CursorAt:  cursorAt,
		CursorID:  cursorID,
	})
	return rows, pgxutil.MapErr(err)
}

type TimelineEntry struct {
	TweetID string
	SortAt  time.Time
}

func (s *Service) GetUserReplies(ctx context.Context, userID string, limit int32, cursor *string) ([]TimelineEntry, error) {
	cursorAt, cursorID, err := decodeProfileCursor(cursor)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.GetUserReplies(ctx, db.GetUserRepliesParams{
		UserID:    userID,
		PageLimit: limit,
		CursorAt:  cursorAt,
		CursorID:  cursorID,
	})
	if err != nil {
		return nil, pgxutil.MapErr(err)
	}
	out := make([]TimelineEntry, len(rows))
	for i, r := range rows {
		out[i] = TimelineEntry{TweetID: r.TweetID, SortAt: r.SortAt}
	}
	return out, nil
}

func (s *Service) GetUserMedia(ctx context.Context, userID string, limit int32, cursor *string) ([]TimelineEntry, error) {
	cursorAt, cursorID, err := decodeProfileCursor(cursor)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.GetUserMedia(ctx, db.GetUserMediaParams{
		UserID:    userID,
		PageLimit: limit,
		CursorAt:  cursorAt,
		CursorID:  cursorID,
	})
	if err != nil {
		return nil, pgxutil.MapErr(err)
	}
	out := make([]TimelineEntry, len(rows))
	for i, r := range rows {
		out[i] = TimelineEntry{TweetID: r.TweetID, SortAt: r.SortAt}
	}
	return out, nil
}

// Cursor sorts on likes.created_at, not tweet creation.
func (s *Service) GetUserLikes(ctx context.Context, userID string, limit int32, cursor *string) ([]TimelineEntry, error) {
	cursorAt, cursorID, err := decodeProfileCursor(cursor)
	if err != nil {
		return nil, err
	}
	rows, err := s.store.GetUserLikes(ctx, db.GetUserLikesParams{
		UserID:    userID,
		PageLimit: limit,
		CursorAt:  cursorAt,
		CursorID:  cursorID,
	})
	if err != nil {
		return nil, pgxutil.MapErr(err)
	}
	out := make([]TimelineEntry, len(rows))
	for i, r := range rows {
		out[i] = TimelineEntry{TweetID: r.TweetID, SortAt: r.SortAt}
	}
	return out, nil
}

func EncodeProfileCursor(sortAt time.Time, itemID string) string {
	return strconv.FormatInt(sortAt.UnixMicro(), 10) + "|" + itemID
}

func decodeProfileCursor(cursor *string) (pgtype.Timestamptz, *string, error) {
	if cursor == nil || *cursor == "" {
		return pgtype.Timestamptz{}, nil, nil
	}
	parts := strings.SplitN(*cursor, "|", 2)
	if len(parts) != 2 {
		return pgtype.Timestamptz{}, nil, httperr.New(http.StatusBadRequest, "INVALID_CURSOR", "invalid cursor")
	}
	micros, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return pgtype.Timestamptz{}, nil, httperr.New(http.StatusBadRequest, "INVALID_CURSOR", "invalid cursor")
	}
	ts := pgtype.Timestamptz{Time: time.UnixMicro(micros), Valid: true}
	id := parts[1]
	return ts, &id, nil
}

// Counts are NOT included here; they come from Redis tweet:counts:{id}.
func (s *Service) GetInteractionsByIDs(ctx context.Context, ids []string, viewerID string) ([]db.GetInteractionsByIDsRow, error) {
	rows, err := s.store.GetInteractionsByIDs(ctx, db.GetInteractionsByIDsParams{
		TweetIds: ids,
		ViewerID: viewerID,
	})
	return rows, pgxutil.MapErr(err)
}

func (s *Service) incrCount(ctx context.Context, tweetID, field string, delta int64) {
	if s.rdb == nil {
		return
	}
	if err := s.rdb.HIncrBy(ctx, "tweet:counts:"+tweetID, field, delta).Err(); err != nil {
		s.log.WarnContext(ctx, "incrCount", "tweet_id", tweetID, "field", field, "err", err)
	}
}

// On cache miss returns a zero AuthorInfo with only ID set.
func (s *Service) FetchAuthor(ctx context.Context, id string) AuthorInfo {
	if s.rdb == nil {
		return AuthorInfo{ID: id}
	}
	fields, err := s.rdb.HGetAll(ctx, "user:snapshot:"+id).Result()
	if err != nil || len(fields) == 0 {
		return AuthorInfo{ID: id}
	}
	return AuthorInfo{
		ID:          id,
		Username:    fields["username"],
		DisplayName: fields["display_name"],
		AvatarURL:   fields["avatar_url"],
	}
}

func (s *Service) FetchAuthors(ctx context.Context, ids []string) map[string]AuthorInfo {
	unique := make(map[string]struct{}, len(ids))
	for _, id := range ids {
		if id != "" {
			unique[id] = struct{}{}
		}
	}
	result := make(map[string]AuthorInfo, len(unique))
	if s.rdb == nil || len(unique) == 0 {
		return result
	}

	type entry struct {
		id  string
		cmd *redis.MapStringStringCmd
	}
	cmds := make([]entry, 0, len(unique))
	pipe := s.rdb.Pipeline()
	for id := range unique {
		cmds = append(cmds, entry{id: id, cmd: pipe.HGetAll(ctx, "user:snapshot:"+id)})
	}
	if _, err := pipe.Exec(ctx); err != nil {
		s.log.WarnContext(ctx, "fetchAuthors: pipeline", "err", err)
	}
	for _, e := range cmds {
		fields, err := e.cmd.Result()
		if err != nil || len(fields) == 0 {
			result[e.id] = AuthorInfo{ID: e.id}
			continue
		}
		result[e.id] = AuthorInfo{
			ID:          e.id,
			Username:    fields["username"],
			DisplayName: fields["display_name"],
			AvatarURL:   fields["avatar_url"],
		}
	}
	return result
}

// At-least-once: offsets commit only after the Redis write succeeds. Redis HSET is
// idempotent, so redelivery on transient failure is safe.
func (s *Service) RunUserSnapshotConsumer(ctx context.Context, r *kafka.Reader) {
	for {
		msg, err := r.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			s.log.ErrorContext(ctx, "user snapshot consumer: fetch", "err", err)
			continue
		}
		var dispatchErr error
		switch msg.Topic {
		case events.TopicUserCreated:
			var evt events.UserCreatedEvent
			if err := json.Unmarshal(msg.Value, &evt); err != nil {
				s.log.ErrorContext(ctx, "user snapshot: unmarshal user.created", "err", err)
				// poison — fall through to commit
			} else {
				dispatchErr = s.writeUserSnapshot(ctx, evt.UserID, evt.Username, evt.DisplayName, evt.AvatarURL)
			}
		case events.TopicUserUpdated:
			var evt events.UserUpdatedEvent
			if err := json.Unmarshal(msg.Value, &evt); err != nil {
				s.log.ErrorContext(ctx, "user snapshot: unmarshal user.updated", "err", err)
			} else {
				dispatchErr = s.writeUserSnapshot(ctx, evt.UserID, evt.Username, evt.DisplayName, evt.AvatarURL)
			}
		}
		if dispatchErr != nil {
			s.log.ErrorContext(ctx, "user snapshot: write failed; will retry", "topic", msg.Topic, "offset", msg.Offset, "err", dispatchErr)
			continue
		}
		if err := r.CommitMessages(ctx, msg); err != nil {
			s.log.ErrorContext(ctx, "user snapshot consumer: commit", "err", err)
		}
	}
}

func (s *Service) writeUserSnapshot(ctx context.Context, userID, username, displayName, avatarURL string) error {
	if s.rdb == nil {
		return nil
	}
	return s.rdb.HSet(ctx, "user:snapshot:"+userID,
		"username", username,
		"display_name", displayName,
		"avatar_url", avatarURL,
	).Err()
}

// GetPendingOutbox uses FOR UPDATE SKIP LOCKED so concurrent flushers cannot claim the same rows.
func (s *Service) FlushOutbox(ctx context.Context) error {
	if s.kb == nil {
		return nil
	}
	return s.store.ExecTx(ctx, func(q db.Querier) error {
		rows, err := q.GetPendingOutbox(ctx)
		if err != nil {
			return fmt.Errorf("outbox: get pending: %w", err)
		}
		metrics.OutboxDepth.WithLabelValues("tweet-service").Set(float64(len(rows)))
		if len(rows) == 0 {
			return nil
		}
		msgs := make([]kafka.Message, 0, len(rows))
		for _, row := range rows {
			headers := []kafka.Header{{Key: outbox.EventIDHeader, Value: []byte(strconv.FormatInt(row.ID, 10))}}
			if row.TraceID != "" {
				// row.TraceID stores the full W3C traceparent value captured
				// at enqueue time. Written under the standard `traceparent`
				// header so consumers picking up via the OTel propagator
				// become children of the producer span.
				headers = append(headers, kafka.Header{Key: tracing.TraceParentHeader, Value: []byte(row.TraceID)})
			}
			msgs = append(msgs, kafka.Message{Topic: row.Topic, Key: []byte(row.Key), Value: row.Payload, Headers: headers})
		}
		if err := s.kb.WriteMessages(ctx, msgs...); err != nil {
			return fmt.Errorf("outbox: kafka write: %w", err)
		}
		for _, row := range rows {
			if err := q.MarkOutboxSent(ctx, row.ID); err != nil {
				s.log.ErrorContext(ctx, "outbox: mark sent", "id", row.ID, "err", err)
			}
		}
		return nil
	})
}

func (s *Service) outboxEnqueue(ctx context.Context, q db.Querier, topic, key string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("outbox marshal: %w", err)
	}
	return q.InsertOutbox(ctx, db.InsertOutboxParams{
		Topic:   topic,
		Key:     key,
		Payload: b,
		TraceID: tracing.TraceParentFromContext(ctx),
	})
}

