package grpcretry_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/twitter/shared/grpcretry"
)

// fakeInvoker scripts a sequence of errors for successive attempts, counting calls.
type fakeInvoker struct {
	results []error
	calls   atomic.Int32
}

func (f *fakeInvoker) invoke(_ context.Context, _ string, _, _ any, _ *grpc.ClientConn, _ ...grpc.CallOption) error {
	n := f.calls.Add(1)
	if int(n) > len(f.results) {
		return f.results[len(f.results)-1]
	}
	return f.results[n-1]
}

func newInterceptor(maxAttempts int) grpc.UnaryClientInterceptor {
	// Sub-millisecond delays so the suite runs fast — jitter logic is the same.
	return grpcretry.UnaryClientInterceptor(grpcretry.Options{
		MaxAttempts: maxAttempts,
		BaseDelay:   100 * time.Microsecond,
		MaxDelay:    500 * time.Microsecond,
	})
}

func TestRetry_SuccessOnFirstTry(t *testing.T) {
	inv := &fakeInvoker{results: []error{nil}}
	icpt := newInterceptor(3)
	if err := icpt(context.Background(), "/svc/Method", nil, nil, nil, inv.invoke); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := inv.calls.Load(); got != 1 {
		t.Errorf("calls = %d, want 1", got)
	}
}

func TestRetry_RecoversAfterTransient(t *testing.T) {
	inv := &fakeInvoker{results: []error{
		status.Error(codes.Unavailable, "down"),
		status.Error(codes.Unavailable, "still down"),
		nil,
	}}
	icpt := newInterceptor(3)
	if err := icpt(context.Background(), "/svc/Method", nil, nil, nil, inv.invoke); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got := inv.calls.Load(); got != 3 {
		t.Errorf("calls = %d, want 3", got)
	}
}

func TestRetry_GivesUpAfterMaxAttempts(t *testing.T) {
	inv := &fakeInvoker{results: []error{status.Error(codes.Unavailable, "down")}}
	icpt := newInterceptor(3)
	err := icpt(context.Background(), "/svc/Method", nil, nil, nil, inv.invoke)
	if status.Code(err) != codes.Unavailable {
		t.Fatalf("expected Unavailable, got %v", err)
	}
	if got := inv.calls.Load(); got != 3 {
		t.Errorf("calls = %d, want 3", got)
	}
}

func TestRetry_NonRetryableReturnsImmediately(t *testing.T) {
	inv := &fakeInvoker{results: []error{status.Error(codes.NotFound, "no such")}}
	icpt := newInterceptor(3)
	err := icpt(context.Background(), "/svc/Method", nil, nil, nil, inv.invoke)
	if status.Code(err) != codes.NotFound {
		t.Fatalf("expected NotFound, got %v", err)
	}
	if got := inv.calls.Load(); got != 1 {
		t.Errorf("calls = %d, want 1 (no retry on non-retryable code)", got)
	}
}

func TestRetry_StopsOnContextCancel(t *testing.T) {
	inv := &fakeInvoker{results: []error{status.Error(codes.Unavailable, "down")}}
	icpt := grpcretry.UnaryClientInterceptor(grpcretry.Options{
		MaxAttempts: 5,
		BaseDelay:   50 * time.Millisecond,
		MaxDelay:    100 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already cancelled — interceptor must short-circuit before invoking.

	err := icpt(ctx, "/svc/Method", nil, nil, nil, inv.invoke)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got %v", err)
	}
	if got := inv.calls.Load(); got != 0 {
		t.Errorf("calls = %d, want 0 (must not invoke when ctx already cancelled)", got)
	}
}
