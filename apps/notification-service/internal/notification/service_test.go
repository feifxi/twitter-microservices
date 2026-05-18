package notification_test

import (
	"context"
	"log/slog"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/twitter/notification-service/db/sqlc"
	"github.com/twitter/notification-service/internal/notification"
)

// stubStore implements db.Store. CreateNotification records calls and
// processed tracks dedup state across calls — both are read by tests to
// verify the atomic dedup-then-insert behaviour.
type stubStore struct {
	created   []db.CreateNotificationParams
	processed map[int64]bool
}

func (s *stubStore) CreateNotification(_ context.Context, arg db.CreateNotificationParams) (db.Notification, error) {
	s.created = append(s.created, arg)
	return db.Notification{
		ID:      arg.ID,
		UserID:  arg.UserID,
		ActorID: arg.ActorID,
		Type:    arg.Type,
		TweetID: arg.TweetID,
	}, nil
}

func (s *stubStore) GetNotificationsByUserID(_ context.Context, _ db.GetNotificationsByUserIDParams) ([]db.Notification, error) {
	return nil, nil
}
func (s *stubStore) MarkAllNotificationsRead(_ context.Context, _ string) error { return nil }
func (s *stubStore) MarkNotificationRead(_ context.Context, _ db.MarkNotificationReadParams) (db.Notification, error) {
	return db.Notification{}, nil
}
func (s *stubStore) CountUnreadNotifications(_ context.Context, _ string) (int32, error) {
	return 0, nil
}
func (s *stubStore) IsEventProcessed(_ context.Context, eventID int64) (bool, error) {
	return s.processed[eventID], nil
}
func (s *stubStore) MarkEventProcessed(_ context.Context, eventID int64) error {
	if s.processed == nil {
		s.processed = map[int64]bool{}
	}
	s.processed[eventID] = true
	return nil
}
func (s *stubStore) PruneProcessedEvents(_ context.Context, _ pgtype.Interval) (int64, error) {
	return 0, nil
}
func (s *stubStore) ExecTx(_ context.Context, fn func(db.Querier) error, afterCommit ...func()) error {
	if err := fn(s); err != nil {
		return err
	}
	for _, f := range afterCommit {
		f()
	}
	return nil
}
func (s *stubStore) Ping(_ context.Context) error { return nil }
func (s *stubStore) DeleteRetweetNotification(_ context.Context, _ db.DeleteRetweetNotificationParams) error {
	return nil
}

func newTestService(t *testing.T) (*notification.Service, *stubStore) {
	t.Helper()
	store := &stubStore{}
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	hub := notification.NewHub(nil, log)
	return notification.New(store, hub, log), store
}

func TestCreateFromLike_SkipsSelf(t *testing.T) {
	svc, store := newTestService(t)
	if err := svc.CreateFromLike(context.Background(), 0, "usr_A", "usr_A", "tw_1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.created) != 0 {
		t.Errorf("expected no notification when actor == author, got %d", len(store.created))
	}
}

func TestCreateFromLike_CreatesForOtherUser(t *testing.T) {
	svc, store := newTestService(t)
	if err := svc.CreateFromLike(context.Background(), 0, "usr_liker", "usr_author", "tw_1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.created) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(store.created))
	}
	n := store.created[0]
	if n.UserID != "usr_author" {
		t.Errorf("notification.user_id = %q, want %q", n.UserID, "usr_author")
	}
	if n.ActorID != "usr_liker" {
		t.Errorf("notification.actor_id = %q, want %q", n.ActorID, "usr_liker")
	}
	if n.Type != string(db.NotifTypeLike) {
		t.Errorf("notification.type = %q, want %q", n.Type, db.NotifTypeLike)
	}
}

func TestCreateFromRetweet_SkipsSelf(t *testing.T) {
	svc, store := newTestService(t)
	if err := svc.CreateFromRetweet(context.Background(), 0, "usr_A", "usr_A", "tw_1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.created) != 0 {
		t.Errorf("expected no notification when retweeter == original author, got %d", len(store.created))
	}
}

func TestCreateFromRetweet_CreatesForOtherUser(t *testing.T) {
	svc, store := newTestService(t)
	tweetID := "tw_1"
	if err := svc.CreateFromRetweet(context.Background(), 0, "usr_retweeter", "usr_author", tweetID); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.created) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(store.created))
	}
	n := store.created[0]
	if n.Type != string(db.NotifTypeRetweet) {
		t.Errorf("notification.type = %q, want %q", n.Type, db.NotifTypeRetweet)
	}
	if n.TweetID == nil || *n.TweetID != tweetID {
		t.Errorf("notification.tweet_id = %v, want %q", n.TweetID, tweetID)
	}
}

func TestCreateFromReply_SkipsSelf(t *testing.T) {
	svc, store := newTestService(t)
	if err := svc.CreateFromReply(context.Background(), 0, "usr_A", "usr_A", "tw_1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.created) != 0 {
		t.Errorf("expected no notification when replier == parent author, got %d", len(store.created))
	}
}

func TestCreateFromReply_CreatesForOtherUser(t *testing.T) {
	svc, store := newTestService(t)
	if err := svc.CreateFromReply(context.Background(), 0, "usr_replier", "usr_author", "tw_parent"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.created) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(store.created))
	}
	n := store.created[0]
	if n.Type != string(db.NotifTypeReply) {
		t.Errorf("notification.type = %q, want %q", n.Type, db.NotifTypeReply)
	}
}

func TestCreateFromFollow_AlwaysCreates(t *testing.T) {
	svc, store := newTestService(t)
	if err := svc.CreateFromFollow(context.Background(), 0, "usr_follower", "usr_followee"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(store.created) != 1 {
		t.Fatalf("expected 1 notification, got %d", len(store.created))
	}
	n := store.created[0]
	if n.Type != string(db.NotifTypeFollow) {
		t.Errorf("notification.type = %q, want %q", n.Type, db.NotifTypeFollow)
	}
	if n.TweetID != nil {
		t.Errorf("follow notification should have nil tweet_id, got %v", n.TweetID)
	}
}

// TestCreateFromLike_DedupsByEventID is the regression test for the atomic
// dedup fix: delivering the same outbox event ID twice must produce exactly
// one notification, even though the consumer hands the event to the service
// twice (simulating a redelivery after a crash between dispatch and offset
// commit).
//
// Before the fix, dedup tracking lived in a separate operation outside the
// notification-insert tx, so a redelivery created a second notification.
// After the fix, MarkEventProcessed runs inside the same ExecTx — the second
// call sees IsEventProcessed == true and returns without inserting.
func TestCreateFromLike_DedupsByEventID(t *testing.T) {
	svc, store := newTestService(t)
	const eventID int64 = 42
	for range 2 {
		if err := svc.CreateFromLike(context.Background(), eventID, "usr_liker", "usr_author", "tw_1"); err != nil {
			t.Fatalf("CreateFromLike: %v", err)
		}
	}
	if len(store.created) != 1 {
		t.Errorf("expected exactly 1 notification for duplicate event %d, got %d", eventID, len(store.created))
	}
	if !store.processed[eventID] {
		t.Errorf("expected event %d to be marked processed", eventID)
	}
}

// TestCreateFromLike_NoEventID_NoDedup verifies that when the consumer
// passes eventID=0 (tests, pre-dedup messages), the service skips the
// dedup machinery entirely — every call produces a notification.
func TestCreateFromLike_NoEventID_NoDedup(t *testing.T) {
	svc, store := newTestService(t)
	for range 2 {
		if err := svc.CreateFromLike(context.Background(), 0, "usr_liker", "usr_author", "tw_1"); err != nil {
			t.Fatalf("CreateFromLike: %v", err)
		}
	}
	if len(store.created) != 2 {
		t.Errorf("expected 2 notifications when eventID=0, got %d", len(store.created))
	}
}
