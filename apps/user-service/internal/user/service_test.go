package user_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/twitter/shared/httperr"
	db "github.com/twitter/user-service/db/sqlc"
	"github.com/twitter/user-service/internal/user"
)

// stubStore implements db.Store. Only the methods exercised by each test are
// non-trivial; the rest return zero values so the stub compiles without noise.
type stubStore struct {
	users           map[string]db.User
	provisionedArgs []db.ProvisionUserParams
	followArgs      []db.CreateFollowParams
	followResult    pgconn.CommandTag // controls RowsAffected for CreateFollow
}

func newStubStore() *stubStore {
	return &stubStore{users: make(map[string]db.User)}
}

func (s *stubStore) GetUserByID(_ context.Context, id string) (db.User, error) {
	u, ok := s.users[id]
	if !ok {
		return db.User{}, pgx.ErrNoRows
	}
	return u, nil
}

func (s *stubStore) ProvisionUser(_ context.Context, arg db.ProvisionUserParams) (db.User, error) {
	s.provisionedArgs = append(s.provisionedArgs, arg)
	if _, exists := s.users[arg.ID]; exists {
		return db.User{}, &pgconn.PgError{Code: "23505"}
	}
	u := db.User{ID: arg.ID, Email: arg.Email, DisplayName: arg.DisplayName, CreatedAt: time.Now()}
	s.users[arg.ID] = u
	return u, nil
}

func (s *stubStore) IsFollowing(_ context.Context, arg db.IsFollowingParams) (bool, error) {
	_, ok := s.users[arg.FollowerID+"->"+arg.FolloweeID]
	return ok, nil
}

func (s *stubStore) CreateFollow(_ context.Context, arg db.CreateFollowParams) (pgconn.CommandTag, error) {
	s.followArgs = append(s.followArgs, arg)
	return s.followResult, nil
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

// ── no-op implementations for unused Querier methods ─────────────────────────

func (s *stubStore) UpdateProfile(_ context.Context, _ db.UpdateProfileParams) (db.User, error) {
	return db.User{}, nil
}
func (s *stubStore) DeleteFollow(_ context.Context, _ db.DeleteFollowParams) error { return nil }
func (s *stubStore) IncrementFollowerCount(_ context.Context, _ string) error      { return nil }
func (s *stubStore) DecrementFollowerCount(_ context.Context, _ string) error      { return nil }
func (s *stubStore) IncrementFollowingCount(_ context.Context, _ string) error     { return nil }
func (s *stubStore) DecrementFollowingCount(_ context.Context, _ string) error     { return nil }
func (s *stubStore) GetFollowerIDs(_ context.Context, _ string) ([]string, error)  { return nil, nil }
func (s *stubStore) GetFollowingIDs(_ context.Context, _ string) ([]string, error) { return nil, nil }
func (s *stubStore) GetFollowState(_ context.Context, _ db.GetFollowStateParams) ([]string, error) {
	return nil, nil
}
func (s *stubStore) ListFollowers(_ context.Context, _ db.ListFollowersParams) ([]db.ListFollowersRow, error) {
	return nil, nil
}
func (s *stubStore) ListFollowing(_ context.Context, _ db.ListFollowingParams) ([]db.ListFollowingRow, error) {
	return nil, nil
}
func (s *stubStore) ListUserSuggestions(_ context.Context, _ db.ListUserSuggestionsParams) ([]db.ListUserSuggestionsRow, error) {
	return nil, nil
}
func (s *stubStore) ListAllUserCounts(_ context.Context) ([]db.ListAllUserCountsRow, error) {
	return nil, nil
}
func (s *stubStore) InsertOutbox(_ context.Context, _ db.InsertOutboxParams) error { return nil }
func (s *stubStore) GetPendingOutbox(_ context.Context) ([]db.GetPendingOutboxRow, error) {
	return nil, nil
}
func (s *stubStore) MarkOutboxSent(_ context.Context, _ int64) error { return nil }
func (s *stubStore) Ping(_ context.Context) error                    { return nil }

func newTestService(t *testing.T, store db.Store) *user.Service {
	t.Helper()
	log := slog.New(slog.NewTextHandler(os.Stderr, nil))
	return user.New(store, nil, nil, nil, log)
}

// ── Follow ────────────────────────────────────────────────────────────────────

func TestFollow_SelfFollowReturnsError(t *testing.T) {
	svc := newTestService(t, newStubStore())
	err := svc.Follow(context.Background(), "usr_A", "usr_A")
	var e *httperr.AppError
	if !errors.As(err, &e) || e.Code != "SELF_FOLLOW" {
		t.Errorf("Follow(self) = %v, want SELF_FOLLOW apierr", err)
	}
}

func TestFollow_UnknownFolloweeReturnsNotFound(t *testing.T) {
	svc := newTestService(t, newStubStore())
	err := svc.Follow(context.Background(), "usr_A", "usr_unknown")
	if !errors.Is(err, httperr.ErrNotFound) {
		t.Errorf("Follow(unknown) = %v, want ErrNotFound", err)
	}
}

func TestFollow_AlreadyFollowingIsIdempotent(t *testing.T) {
	store := newStubStore()
	store.users["usr_B"] = db.User{ID: "usr_B"}
	// RowsAffected() == 0 signals duplicate (ON CONFLICT DO NOTHING)
	store.followResult = pgconn.NewCommandTag("INSERT 0 0")
	svc := newTestService(t, store)

	if err := svc.Follow(context.Background(), "usr_A", "usr_B"); err != nil {
		t.Errorf("Follow (already following) = %v, want nil", err)
	}
}

// ── GetProfile ────────────────────────────────────────────────────────────────

func TestGetProfile_IsFollowingFalseWhenViewerEqualsTarget(t *testing.T) {
	store := newStubStore()
	store.users["usr_A"] = db.User{ID: "usr_A", Email: "a@example.com"}
	svc := newTestService(t, store)

	p, err := svc.GetProfile(context.Background(), "usr_A", "usr_A")
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if p.IsFollowing {
		t.Error("is_following should be false when viewer == target")
	}
}

func TestGetProfile_IsFollowingFalseWhenNoViewer(t *testing.T) {
	store := newStubStore()
	store.users["usr_A"] = db.User{ID: "usr_A", Email: "a@example.com"}
	svc := newTestService(t, store)

	p, err := svc.GetProfile(context.Background(), "usr_A", "")
	if err != nil {
		t.Fatalf("GetProfile: %v", err)
	}
	if p.IsFollowing {
		t.Error("is_following should be false when viewerID is empty")
	}
}

func TestGetProfile_NotFoundReturnsError(t *testing.T) {
	svc := newTestService(t, newStubStore())
	_, err := svc.GetProfile(context.Background(), "usr_missing", "usr_viewer")
	if !errors.Is(err, httperr.ErrNotFound) {
		t.Errorf("GetProfile(missing) = %v, want ErrNotFound", err)
	}
}

// ── Provision ─────────────────────────────────────────────────────────────────

func TestProvision_NormalizesEmail(t *testing.T) {
	store := newStubStore()
	svc := newTestService(t, store)

	_, _, err := svc.Provision(context.Background(), "sub-123", "  ALICE@EXAMPLE.COM  ", "Alice")
	if err != nil {
		t.Fatalf("Provision: %v", err)
	}
	if len(store.provisionedArgs) != 1 {
		t.Fatalf("expected 1 provision call, got %d", len(store.provisionedArgs))
	}
	got := store.provisionedArgs[0].Email
	if got != "alice@example.com" {
		t.Errorf("email = %q, want %q", got, "alice@example.com")
	}
}

func TestProvision_IdempotentOnDuplicateSub(t *testing.T) {
	store := newStubStore()
	store.users["sub-123"] = db.User{ID: "sub-123", Email: "alice@example.com"}
	svc := newTestService(t, store)

	// stub ProvisionUser to simulate PK conflict by returning ErrConflict
	// Re-provision should return existing user with created=false
	u, created, err := svc.Provision(context.Background(), "sub-123", "alice@example.com", "Alice")
	if err != nil {
		t.Fatalf("re-provision: %v", err)
	}
	if created {
		t.Error("created should be false on re-provision")
	}
	if u.ID != "sub-123" {
		t.Errorf("user.id = %q, want %q", u.ID, "sub-123")
	}
}
