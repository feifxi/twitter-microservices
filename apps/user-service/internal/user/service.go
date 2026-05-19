package user

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
	"github.com/twitter/shared/events"
	"github.com/twitter/shared/httperr"
	"github.com/twitter/shared/metrics"
	"github.com/twitter/shared/outbox"
	"github.com/twitter/shared/pgxutil"
	"github.com/twitter/shared/ptr"
	"github.com/twitter/shared/tracing"
	db "github.com/twitter/user-service/db/sqlc"
	"github.com/twitter/user-service/internal/keycloak"
)

type Profile struct {
	db.User
	IsFollowing bool
}

// Nil fields mean "do not change".
type UpdateProfileParams struct {
	Username       *string
	DisplayName    *string
	AvatarURL      *string
	HeaderImageURL *string
	Bio            *string
	WebsiteURL     *string
	Location       *string
}

type Service struct {
	store db.Store
	rdb   *redis.Client
	kb    *kafka.Writer
	kc    keycloak.Admin
	log   *slog.Logger
}

func New(store db.Store, rdb *redis.Client, kb *kafka.Writer, kc keycloak.Admin, log *slog.Logger) *Service {
	return &Service{store: store, rdb: rdb, kb: kb, kc: kc, log: log}
}

// userCountsKey holds follower_count + following_count for fast cross-service
// reads. user-service is the single writer — incremented on follow/unfollow
// after the Postgres tx commits. RefreshAllCounts on boot reconciles drift
// (fresh Redis volume, mid-write crash).
const userCountsKey = "user:counts:%s"

// HSets both counts to the values currently in Postgres. Idempotent — running
// it twice produces the same state. Nil rdb is a no-op (unit tests).
func (s *Service) RefreshAllCounts(ctx context.Context) error {
	if s.rdb == nil {
		return nil
	}
	users, err := s.store.ListAllUserCounts(ctx)
	if err != nil {
		return err
	}
	pipe := s.rdb.Pipeline()
	for _, u := range users {
		pipe.HSet(ctx, fmt.Sprintf(userCountsKey, u.ID), map[string]any{
			"follower_count":  u.FollowerCount,
			"following_count": u.FollowingCount,
		})
	}
	_, err = pipe.Exec(ctx)
	return err
}

// incrCounts adjusts follower / following counts in Redis after a successful
// follow or unfollow. Best-effort: a Redis blip leaves Postgres authoritative,
// and the next RefreshAllCounts (boot) reconciles. Eventually consistent.
func (s *Service) incrCounts(ctx context.Context, followerID, followeeID string, delta int64) {
	if s.rdb == nil {
		return
	}
	pipe := s.rdb.Pipeline()
	pipe.HIncrBy(ctx, fmt.Sprintf(userCountsKey, followerID), "following_count", delta)
	pipe.HIncrBy(ctx, fmt.Sprintf(userCountsKey, followeeID), "follower_count", delta)
	if _, err := pipe.Exec(ctx); err != nil {
		s.log.WarnContext(ctx, "user:counts hincrby", "follower", followerID, "followee", followeeID, "delta", delta, "err", err)
	}
}

// Provision is idempotent on duplicate sub; an email conflict with a different sub returns ErrConflict.
func (s *Service) Provision(ctx context.Context, sub, email, displayName string) (db.User, bool, error) {
	email = strings.ToLower(strings.TrimSpace(email))

	var u db.User
	err := s.store.ExecTx(ctx, func(q db.Querier) error {
		var err error
		u, err = q.ProvisionUser(ctx, db.ProvisionUserParams{
			ID:          sub,
			Email:       email,
			DisplayName: ptr.NonEmpty(displayName),
		})
		if err != nil {
			return pgxutil.MapErr(err)
		}
		username := ""
		if u.Username != nil {
			username = *u.Username
		}
		displayName := ""
		if u.DisplayName != nil {
			displayName = *u.DisplayName
		}
		return s.outboxEnqueue(ctx, q, events.TopicUserCreated, u.ID, events.UserCreatedEvent{
			UserID:      u.ID,
			Username:    username,
			DisplayName: displayName,
			Bio:         ptr.Deref(u.Bio),
			AvatarURL:   ptr.Deref(u.AvatarUrl),
			CreatedAt:   u.CreatedAt,
		})
	})
	if err != nil {
		if errors.Is(err, httperr.ErrConflict) {
			// Distinguish PK conflict (same sub, idempotent) from email UNIQUE conflict (different sub).
			existing, err := s.store.GetUserByID(ctx, sub)
			if err == nil {
				return existing, false, nil
			}
			return db.User{}, false, httperr.ErrConflict
		}
		return db.User{}, false, err
	}
	// Seed user:counts so cross-service readers don't need a fallback path.
	if s.rdb != nil {
		if err := s.rdb.HSet(ctx, fmt.Sprintf(userCountsKey, u.ID), map[string]any{
			"follower_count":  0,
			"following_count": 0,
		}).Err(); err != nil {
			s.log.WarnContext(ctx, "seed user:counts", "user_id", u.ID, "err", err)
		}
	}
	return u, true, nil
}

func (s *Service) GetUserByID(ctx context.Context, userID string) (db.User, error) {
	u, err := s.store.GetUserByID(ctx, userID)
	return u, pgxutil.MapErr(err)
}

func (s *Service) GetProfile(ctx context.Context, targetID, viewerID string) (*Profile, error) {
	u, err := s.store.GetUserByID(ctx, targetID)
	if err != nil {
		return nil, pgxutil.MapErr(err)
	}
	var isFollowing bool
	if viewerID != "" && viewerID != targetID {
		isFollowing, err = s.store.IsFollowing(ctx, db.IsFollowingParams{
			FollowerID: viewerID,
			FolloweeID: targetID,
		})
		if err != nil {
			return nil, pgxutil.MapErr(err)
		}
	}
	return &Profile{User: u, IsFollowing: isFollowing}, nil
}

func (s *Service) UpdateProfile(ctx context.Context, userID string, p UpdateProfileParams) (db.User, error) {
	var u db.User
	err := s.store.ExecTx(ctx, func(q db.Querier) error {
		var err error
		u, err = q.UpdateProfile(ctx, db.UpdateProfileParams{
			ID:             userID,
			Username:       p.Username,
			DisplayName:    p.DisplayName,
			AvatarUrl:      p.AvatarURL,
			HeaderImageUrl: p.HeaderImageURL,
			Bio:            p.Bio,
			WebsiteUrl:     p.WebsiteURL,
			Location:       p.Location,
		})
		if err != nil {
			return pgxutil.MapErr(err)
		}
		return s.outboxEnqueue(ctx, q, events.TopicUserUpdated, userID, events.UserUpdatedEvent{
			UserID:      u.ID,
			Username:    ptr.Deref(u.Username),
			DisplayName: ptr.Deref(u.DisplayName),
			Bio:         ptr.Deref(u.Bio),
			AvatarURL:   ptr.Deref(u.AvatarUrl),
		})
	}, func() {
		// After-commit: external HTTP, stale KC claim self-heals on next token refresh.
		if p.Username != nil && u.Username != nil {
			if err := s.kc.UpdateUsername(ctx, u.ID, *u.Username); err != nil {
				s.log.WarnContext(ctx, "keycloak username sync failed", "user_id", userID, "err", err)
			}
		}
	})
	if err != nil {
		return db.User{}, err
	}
	return u, nil
}

func (s *Service) Follow(ctx context.Context, followerID, followeeID string) error {
	if followerID == followeeID {
		return httperr.New(http.StatusBadRequest, "SELF_FOLLOW", "cannot follow yourself")
	}
	if _, err := s.store.GetUserByID(ctx, followeeID); err != nil {
		return pgxutil.MapErr(err)
	}

	var applied bool
	err := s.store.ExecTx(ctx, func(q db.Querier) error {
		tag, err := q.CreateFollow(ctx, db.CreateFollowParams{
			FollowerID: followerID,
			FolloweeID: followeeID,
		})
		if err != nil {
			return pgxutil.MapErr(err)
		}
		if tag.RowsAffected() == 0 {
			return nil
		}
		applied = true
		if err := q.IncrementFollowerCount(ctx, followeeID); err != nil {
			s.log.WarnContext(ctx, "increment follower count", "user_id", followeeID, "err", err)
		}
		if err := q.IncrementFollowingCount(ctx, followerID); err != nil {
			s.log.WarnContext(ctx, "increment following count", "user_id", followerID, "err", err)
		}
		return s.outboxEnqueue(ctx, q, events.TopicUserFollowed, followeeID, events.UserFollowedEvent{
			FollowerID: followerID,
			FolloweeID: followeeID,
			CreatedAt:  time.Now(),
		})
	})
	if err == nil && applied {
		s.incrCounts(ctx, followerID, followeeID, +1)
	}
	return err
}

func (s *Service) Unfollow(ctx context.Context, followerID, followeeID string) error {
	err := s.store.ExecTx(ctx, func(q db.Querier) error {
		if err := q.DeleteFollow(ctx, db.DeleteFollowParams{
			FollowerID: followerID,
			FolloweeID: followeeID,
		}); err != nil {
			return pgxutil.MapErr(err)
		}
		if err := q.DecrementFollowerCount(ctx, followeeID); err != nil {
			s.log.WarnContext(ctx, "decrement follower count", "user_id", followeeID, "err", err)
		}
		if err := q.DecrementFollowingCount(ctx, followerID); err != nil {
			s.log.WarnContext(ctx, "decrement following count", "user_id", followerID, "err", err)
		}
		return nil
	})
	if err == nil {
		s.incrCounts(ctx, followerID, followeeID, -1)
	}
	return err
}

type UserListItem struct {
	db.User
	IsFollowing bool
}

const (
	defaultListLimit       = 20
	maxListLimit           = 100
	defaultSuggestionLimit = 10
	maxSuggestionLimit     = 30
)

// ID alongside the timestamp handles ties deterministically.
func EncodeFollowCursor(followedAt time.Time, userID string) string {
	return strconv.FormatInt(followedAt.UnixMicro(), 10) + "|" + userID
}

func decodeFollowCursor(cursor *string) (pgtype.Timestamptz, *string, error) {
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
	id := parts[1]
	return pgtype.Timestamptz{Time: time.UnixMicro(micros), Valid: true}, &id, nil
}

func clampLimit(limit, def, max int) int32 {
	if limit <= 0 {
		return int32(def)
	}
	if limit > max {
		return int32(max)
	}
	return int32(limit)
}

// viewerID drives the is_following flag on each returned user.
func (s *Service) ListFollowers(ctx context.Context, userID, viewerID string, cursor *string, limit int) ([]UserListItem, *string, error) {
	cursorAt, cursorID, err := decodeFollowCursor(cursor)
	if err != nil {
		return nil, nil, err
	}
	pageLimit := clampLimit(limit, defaultListLimit, maxListLimit)
	rows, err := s.store.ListFollowers(ctx, db.ListFollowersParams{
		ViewerID:  ptr.NonEmpty(viewerID),
		UserID:    userID,
		CursorAt:  cursorAt,
		CursorID:  cursorID,
		PageLimit: pageLimit,
	})
	if err != nil {
		return nil, nil, pgxutil.MapErr(err)
	}
	items := make([]UserListItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, UserListItem{
			User: db.User{
				ID:             r.ID,
				Email:          r.Email,
				Username:       r.Username,
				DisplayName:    r.DisplayName,
				AvatarUrl:      r.AvatarUrl,
				HeaderImageUrl: r.HeaderImageUrl,
				Bio:            r.Bio,
				WebsiteUrl:     r.WebsiteUrl,
				Location:       r.Location,
				FollowerCount:  r.FollowerCount,
				FollowingCount: r.FollowingCount,
				CreatedAt:      r.CreatedAt,
				UpdatedAt:      r.UpdatedAt,
			},
			IsFollowing: r.IsFollowingViewer,
		})
	}
	var next *string
	if int32(len(rows)) == pageLimit {
		last := rows[len(rows)-1]
		c := EncodeFollowCursor(last.FollowedAt, last.ID)
		next = &c
	}
	return items, next, nil
}

func (s *Service) ListFollowing(ctx context.Context, userID, viewerID string, cursor *string, limit int) ([]UserListItem, *string, error) {
	cursorAt, cursorID, err := decodeFollowCursor(cursor)
	if err != nil {
		return nil, nil, err
	}
	pageLimit := clampLimit(limit, defaultListLimit, maxListLimit)
	rows, err := s.store.ListFollowing(ctx, db.ListFollowingParams{
		ViewerID:  ptr.NonEmpty(viewerID),
		UserID:    userID,
		CursorAt:  cursorAt,
		CursorID:  cursorID,
		PageLimit: pageLimit,
	})
	if err != nil {
		return nil, nil, pgxutil.MapErr(err)
	}
	items := make([]UserListItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, UserListItem{
			User: db.User{
				ID:             r.ID,
				Email:          r.Email,
				Username:       r.Username,
				DisplayName:    r.DisplayName,
				AvatarUrl:      r.AvatarUrl,
				HeaderImageUrl: r.HeaderImageUrl,
				Bio:            r.Bio,
				WebsiteUrl:     r.WebsiteUrl,
				Location:       r.Location,
				FollowerCount:  r.FollowerCount,
				FollowingCount: r.FollowingCount,
				CreatedAt:      r.CreatedAt,
				UpdatedAt:      r.UpdatedAt,
			},
			IsFollowing: r.IsFollowingViewer,
		})
	}
	var next *string
	if int32(len(rows)) == pageLimit {
		last := rows[len(rows)-1]
		c := EncodeFollowCursor(last.FollowedAt, last.ID)
		next = &c
	}
	return items, next, nil
}

// Not paginated — clients re-call to reshuffle.
func (s *Service) ListSuggestions(ctx context.Context, viewerID string, limit int) ([]UserListItem, error) {
	pageLimit := clampLimit(limit, defaultSuggestionLimit, maxSuggestionLimit)
	rows, err := s.store.ListUserSuggestions(ctx, db.ListUserSuggestionsParams{
		ViewerID:  viewerID,
		PageLimit: pageLimit,
	})
	if err != nil {
		return nil, pgxutil.MapErr(err)
	}
	items := make([]UserListItem, 0, len(rows))
	for _, r := range rows {
		items = append(items, UserListItem{
			User: db.User{
				ID:             r.ID,
				Email:          r.Email,
				Username:       r.Username,
				DisplayName:    r.DisplayName,
				AvatarUrl:      r.AvatarUrl,
				HeaderImageUrl: r.HeaderImageUrl,
				Bio:            r.Bio,
				WebsiteUrl:     r.WebsiteUrl,
				Location:       r.Location,
				FollowerCount:  r.FollowerCount,
				FollowingCount: r.FollowingCount,
				CreatedAt:      r.CreatedAt,
				UpdatedAt:      r.UpdatedAt,
			},
			IsFollowing: false, // by construction — query excludes already-followed
		})
	}
	return items, nil
}

func (s *Service) GetFollowerIDs(ctx context.Context, userID string) ([]string, error) {
	ids, err := s.store.GetFollowerIDs(ctx, userID)
	return ids, pgxutil.MapErr(err)
}

func (s *Service) GetFollowingIDs(ctx context.Context, userID string) ([]string, error) {
	ids, err := s.store.GetFollowingIDs(ctx, userID)
	return ids, pgxutil.MapErr(err)
}

// Returned as a map for O(1) lookup; missing keys default to false.
func (s *Service) GetFollowState(ctx context.Context, viewerID string, targetIDs []string) (map[string]bool, error) {
	if viewerID == "" || len(targetIDs) == 0 {
		return map[string]bool{}, nil
	}
	followed, err := s.store.GetFollowState(ctx, db.GetFollowStateParams{
		ViewerID:  viewerID,
		TargetIds: targetIDs,
	})
	if err != nil {
		return nil, pgxutil.MapErr(err)
	}
	out := make(map[string]bool, len(followed))
	for _, id := range followed {
		out[id] = true
	}
	return out, nil
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
		metrics.OutboxDepth.WithLabelValues("user-service").Set(float64(len(rows)))
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
