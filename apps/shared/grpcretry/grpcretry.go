package grpcretry

import (
	"context"
	"math/rand/v2"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Options configures the unary client retry interceptor. Sits inside the
// gobreaker boundary so retries are transparent to the breaker: it sees the
// final outcome (success after retry = success, failure after retry = failure).
type Options struct {
	// Total attempts including the first try. Default 3.
	MaxAttempts int
	// First backoff before attempt #2. Default 50ms.
	BaseDelay time.Duration
	// Hard cap on any single backoff. Default 500ms.
	MaxDelay time.Duration
	// Codes that trigger a retry. Default: Unavailable, DeadlineExceeded.
	// Other codes — including InvalidArgument, NotFound, AlreadyExists,
	// PermissionDenied — are returned as-is on the first attempt.
	RetryCodes []codes.Code
}

func (o *Options) applyDefaults() {
	if o.MaxAttempts <= 0 {
		o.MaxAttempts = 3
	}
	if o.BaseDelay <= 0 {
		o.BaseDelay = 50 * time.Millisecond
	}
	if o.MaxDelay <= 0 {
		o.MaxDelay = 500 * time.Millisecond
	}
	if len(o.RetryCodes) == 0 {
		o.RetryCodes = []codes.Code{codes.Unavailable, codes.DeadlineExceeded}
	}
}

// UnaryClientInterceptor returns an interceptor that retries idempotent calls
// on transient codes with full-jitter exponential backoff (AWS Architecture
// Blog: "Exponential Backoff and Jitter"). All four cross-service gRPC clients
// in this codebase only call read methods, which are safe to retry; if a
// mutation client is added later, exclude it from the dial or pass an empty
// RetryCodes slice.
func UnaryClientInterceptor(opts Options) grpc.UnaryClientInterceptor {
	opts.applyDefaults()
	retryable := make(map[codes.Code]struct{}, len(opts.RetryCodes))
	for _, c := range opts.RetryCodes {
		retryable[c] = struct{}{}
	}
	return func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, callOpts ...grpc.CallOption) error {
		var lastErr error
		for attempt := 0; attempt < opts.MaxAttempts; attempt++ {
			if err := ctx.Err(); err != nil {
				return err
			}
			lastErr = invoker(ctx, method, req, reply, cc, callOpts...)
			if lastErr == nil {
				return nil
			}
			if _, ok := retryable[status.Code(lastErr)]; !ok {
				return lastErr
			}
			if attempt == opts.MaxAttempts-1 {
				return lastErr
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(jitter(opts.BaseDelay, opts.MaxDelay, attempt)):
			}
		}
		return lastErr
	}
}

// Full jitter: sleep ∈ [0, min(maxDelay, base * 2^attempt)). Spreads load
// better than equal-jitter when many clients fail at the same wall-clock tick.
func jitter(base, max time.Duration, attempt int) time.Duration {
	cap := base << attempt
	if cap > max || cap <= 0 {
		cap = max
	}
	return time.Duration(rand.Int64N(int64(cap)))
}
