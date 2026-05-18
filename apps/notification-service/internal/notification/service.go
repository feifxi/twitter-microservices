package notification

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/oklog/ulid/v2"
	"github.com/twitter/shared/pgxutil"
	db "github.com/twitter/notification-service/db/sqlc"
)

type Service struct {
	store db.Store
	hub   *Hub
	log   *slog.Logger
}

func New(store db.Store, hub *Hub, log *slog.Logger) *Service {
	return &Service{store: store, hub: hub, log: log}
}

// The retention window must comfortably exceed Kafka's broker retention so no
// message can be redelivered after its dedup row was pruned.
func (s *Service) RunDedupPrune(ctx context.Context) {
	const (
		interval     = 6 * time.Hour
		olderThanDur = 7 * 24 * time.Hour
	)
	olderThan := pgtype.Interval{Microseconds: olderThanDur.Microseconds(), Valid: true}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			n, err := s.store.PruneProcessedEvents(ctx, olderThan)
			if err != nil {
				s.log.ErrorContext(ctx, "dedup prune failed", "err", err)
				continue
			}
			if n > 0 {
				s.log.InfoContext(ctx, "dedup pruned", "rows", n)
			}
		}
	}
}

// CreateFromLike etc. accept the outbox-row event ID so they can record it
// atomically with the notification insert. eventID == 0 disables dedup
// (used by tests that don't carry the X-Event-ID header).
func (s *Service) CreateFromLike(ctx context.Context, eventID int64, actorID, authorID, tweetID string) error {
	if actorID == authorID {
		return nil
	}
	return s.create(ctx, eventID, authorID, actorID, db.NotifTypeLike, &tweetID)
}

func (s *Service) CreateFromRetweet(ctx context.Context, eventID int64, retweeterID, originalAuthorID, tweetID string) error {
	if retweeterID == originalAuthorID {
		return nil
	}
	return s.create(ctx, eventID, originalAuthorID, retweeterID, db.NotifTypeRetweet, &tweetID)
}

// DeleteFromRetweet removes the "X retweeted you" notification when X
// unretweets. Best-effort: missing rows are not an error (the user may have
// already cleared the notification, or it was never delivered).
func (s *Service) DeleteFromRetweet(ctx context.Context, retweeterID, tweetID string) error {
	return pgxutil.MapErr(s.store.DeleteRetweetNotification(ctx, db.DeleteRetweetNotificationParams{
		ActorID: retweeterID,
		TweetID: &tweetID,
	}))
}

func (s *Service) CreateFromReply(ctx context.Context, eventID int64, replierID, parentAuthorID, parentTweetID string) error {
	if replierID == parentAuthorID {
		return nil
	}
	return s.create(ctx, eventID, parentAuthorID, replierID, db.NotifTypeReply, &parentTweetID)
}

func (s *Service) CreateFromFollow(ctx context.Context, eventID int64, followerID, followeeID string) error {
	return s.create(ctx, eventID, followeeID, followerID, db.NotifTypeFollow, nil)
}

func (s *Service) GetNotifications(ctx context.Context, userID string, cursor *string, limit int32) ([]db.Notification, error) {
	rows, err := s.store.GetNotificationsByUserID(ctx, db.GetNotificationsByUserIDParams{
		UserID:    userID,
		Cursor:    cursor,
		PageLimit: limit,
	})
	return rows, pgxutil.MapErr(err)
}

func (s *Service) CountUnread(ctx context.Context, userID string) (int32, error) {
	n, err := s.store.CountUnreadNotifications(ctx, userID)
	return n, pgxutil.MapErr(err)
}

func (s *Service) MarkAllRead(ctx context.Context, userID string) error {
	return pgxutil.MapErr(s.store.MarkAllNotificationsRead(ctx, userID))
}

func (s *Service) MarkRead(ctx context.Context, notifID, userID string) (db.Notification, error) {
	n, err := s.store.MarkNotificationRead(ctx, db.MarkNotificationReadParams{
		ID:     notifID,
		UserID: userID,
	})
	return n, pgxutil.MapErr(err)
}

// Dedup record and notification row commit atomically; hub.Publish runs after commit.
func (s *Service) create(ctx context.Context, eventID int64, userID, actorID string, t db.NotifType, tweetID *string) error {
	var n db.Notification
	var created bool
	err := s.store.ExecTx(ctx, func(q db.Querier) error {
		if eventID > 0 {
			processed, err := q.IsEventProcessed(ctx, eventID)
			if err != nil {
				return pgxutil.MapErr(err)
			}
			if processed {
				return nil // dedup hit — tx will be a no-op
			}
			if err := q.MarkEventProcessed(ctx, eventID); err != nil {
				return pgxutil.MapErr(err)
			}
		}
		id := "ntf_" + ulid.Make().String()
		row, err := q.CreateNotification(ctx, db.CreateNotificationParams{
			ID:      id,
			UserID:  userID,
			ActorID: actorID,
			Type:    string(t),
			TweetID: tweetID,
		})
		if err != nil {
			return pgxutil.MapErr(err)
		}
		n = row
		created = true
		return nil
	}, func() {
		if created {
			s.hub.Publish(ctx, userID, n)
		}
	})
	return err
}
