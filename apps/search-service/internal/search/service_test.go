package search_test

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"github.com/twitter/search-service/internal/opensearch"
	"github.com/twitter/search-service/internal/search"
)

type stubOS struct {
	tweets     []opensearch.TweetDoc
	users      []opensearch.UserDoc
	lastMode   string
	lastVector []float32
}

func (s *stubOS) SearchTweets(_ context.Context, _ string, vector []float32, mode string, _, _ int) ([]opensearch.TweetDoc, error) {
	s.lastMode = mode
	s.lastVector = vector
	return s.tweets, nil
}

func (s *stubOS) SearchUsers(_ context.Context, _ string, _, _ int) ([]opensearch.UserDoc, error) {
	return s.users, nil
}

type stubEmbed struct {
	vec []float32
	err error
}

func (e *stubEmbed) Embed(_ context.Context, _ string) ([]float32, error) {
	return e.vec, e.err
}

type stubUsers struct {
	counts      map[string]int64
	followState map[string]bool
	countsErr   error
	followErr   error
}

func (u *stubUsers) BatchGetFollowerCounts(_ context.Context, _ []string) (map[string]int64, error) {
	return u.counts, u.countsErr
}
func (u *stubUsers) GetFollowState(_ context.Context, _ string, _ []string) (map[string]bool, error) {
	return u.followState, u.followErr
}

type stubTweets struct {
	authors         map[string]search.AuthorSnapshot
	counts          map[string]search.TweetCounts
	interactions    map[string]search.TweetInteraction
	interactionsErr error
}

func (t *stubTweets) BatchAuthorSnapshots(_ context.Context, _ []string) map[string]search.AuthorSnapshot {
	return t.authors
}
func (t *stubTweets) BatchTweetCounts(_ context.Context, _ []string) map[string]search.TweetCounts {
	return t.counts
}
func (t *stubTweets) GetInteractions(_ context.Context, _ string, _ []string) (map[string]search.TweetInteraction, error) {
	return t.interactions, t.interactionsErr
}

func newSvc(store *stubOS, em *stubEmbed) *search.Service {
	log := slog.New(slog.NewTextHandler(io.Discard, nil))
	return search.New(store, em, &stubUsers{}, &stubTweets{}, log)
}

func makeTweets(n int) []opensearch.TweetDoc {
	out := make([]opensearch.TweetDoc, n)
	for i := range out {
		out[i] = opensearch.TweetDoc{ID: "tw_" + string(rune('a'+i)), CreatedAt: time.Now()}
	}
	return out
}

func TestNormaliseMode_UnknownFallsToHybrid(t *testing.T) {
	for _, bad := range []string{"", "KEYWORD", "fuzzy", "vector"} {
		got := search.ExportedNormaliseMode(bad)
		if got != search.ModeHybrid {
			t.Errorf("normaliseMode(%q) = %q, want %q", bad, got, search.ModeHybrid)
		}
	}
}

func TestNormaliseMode_ValidModesPassThrough(t *testing.T) {
	for _, m := range []string{search.ModeKeyword, search.ModeSemantic, search.ModeHybrid} {
		got := search.ExportedNormaliseMode(m)
		if got != m {
			t.Errorf("normaliseMode(%q) = %q, want %q", m, got, m)
		}
	}
}

func TestDecodeCursor_NilReturnsZero(t *testing.T) {
	if got := search.ExportedDecodeCursor(nil); got != 0 {
		t.Errorf("decodeCursor(nil) = %d, want 0", got)
	}
}

func TestDecodeCursor_ValidString(t *testing.T) {
	s := "40"
	if got := search.ExportedDecodeCursor(&s); got != 40 {
		t.Errorf("decodeCursor(%q) = %d, want 40", s, got)
	}
}

func TestDecodeCursor_InvalidStringReturnsZero(t *testing.T) {
	for _, bad := range []string{"abc", "-1", ""} {
		b := bad
		if got := search.ExportedDecodeCursor(&b); got != 0 {
			t.Errorf("decodeCursor(%q) = %d, want 0", bad, got)
		}
	}
}

func TestCursorRoundTrip(t *testing.T) {
	next := search.ExportedNextCursorFor(40, 20, 20)
	if next == nil {
		t.Fatal("expected non-nil cursor when got == limit")
	}
	if *next != "60" {
		t.Errorf("cursor = %q, want %q", *next, "60")
	}
}

func TestCursorNilOnLastPage(t *testing.T) {
	if next := search.ExportedNextCursorFor(0, 20, 5); next != nil {
		t.Errorf("expected nil cursor on last page, got %q", *next)
	}
}

func TestSearchTweets_KeywordSkipsEmbed(t *testing.T) {
	em := &stubEmbed{vec: []float32{0.1}, err: nil}
	store := &stubOS{tweets: makeTweets(1)}
	svc := newSvc(store, em)

	_, _, effectiveMode, err := svc.SearchTweets(context.Background(), "golang", search.ModeKeyword, "viewer_1", nil, 20)
	if err != nil {
		t.Fatal(err)
	}
	if store.lastVector != nil {
		t.Error("keyword mode should not pass a vector to OpenSearch")
	}
	if effectiveMode != search.ModeKeyword {
		t.Errorf("effective mode = %q, want %q", effectiveMode, search.ModeKeyword)
	}
}

func TestSearchTweets_SemanticPassesVector(t *testing.T) {
	vec := []float32{0.1, 0.2, 0.3}
	em := &stubEmbed{vec: vec}
	store := &stubOS{tweets: makeTweets(2)}
	svc := newSvc(store, em)

	_, _, effectiveMode, err := svc.SearchTweets(context.Background(), "golang", search.ModeSemantic, "viewer_1", nil, 20)
	if err != nil {
		t.Fatal(err)
	}
	if store.lastVector == nil {
		t.Error("semantic mode should pass a vector to OpenSearch")
	}
	if effectiveMode != search.ModeSemantic {
		t.Errorf("effective mode = %q, want %q", effectiveMode, search.ModeSemantic)
	}
}

func TestSearchTweets_EmbedFailureFallsBackToKeyword(t *testing.T) {
	em := &stubEmbed{err: errors.New("invalid api key")}
	store := &stubOS{tweets: makeTweets(1)}
	svc := newSvc(store, em)

	_, _, effectiveMode, err := svc.SearchTweets(context.Background(), "golang", search.ModeSemantic, "viewer_1", nil, 20)
	if err != nil {
		t.Fatal(err)
	}
	if effectiveMode != search.ModeKeyword {
		t.Errorf("expected fallback to keyword, got %q", effectiveMode)
	}
	if store.lastVector != nil {
		t.Error("no vector should be sent to OpenSearch on fallback")
	}
}

func TestSearchTweets_CursorPaginationAdvancesOffset(t *testing.T) {
	em := &stubEmbed{}
	store := &stubOS{tweets: makeTweets(20)}
	svc := newSvc(store, em)

	_, nextCursor, _, err := svc.SearchTweets(context.Background(), "q", search.ModeKeyword, "viewer_1", nil, 20)
	if err != nil {
		t.Fatal(err)
	}
	if nextCursor == nil {
		t.Fatal("expected next_cursor when results == limit")
	}
	if *nextCursor != "20" {
		t.Errorf("next_cursor = %q, want %q", *nextCursor, "20")
	}
}

func TestSearchTweets_UnderLimitHasNoCursor(t *testing.T) {
	em := &stubEmbed{}
	store := &stubOS{tweets: makeTweets(5)}
	svc := newSvc(store, em)

	_, nextCursor, _, err := svc.SearchTweets(context.Background(), "q", search.ModeKeyword, "viewer_1", nil, 20)
	if err != nil {
		t.Fatal(err)
	}
	if nextCursor != nil {
		t.Errorf("expected nil cursor on last page, got %q", *nextCursor)
	}
}

func TestSearchUsers_ReturnsResults(t *testing.T) {
	store := &stubOS{users: []opensearch.UserDoc{{ID: "usr_1", Username: "alice"}}}
	svc := newSvc(store, &stubEmbed{})

	results, _, err := svc.SearchUsers(context.Background(), "alice", "viewer_1", nil, 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 1 || results[0].Doc.Username != "alice" {
		t.Errorf("unexpected results: %v", results)
	}
}
