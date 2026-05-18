package outbox

import (
	"context"
	"log/slog"
	"math/rand"
	"time"
)

// Consumers use this header to dedup idempotently across redeliveries.
const EventIDHeader = "X-Event-ID"

type Flush func(ctx context.Context) error

// Final drain on shutdown uses a 5s deadline so rows written in the last tick still get published.
func Run(ctx context.Context, log *slog.Logger, service string, flush Flush) {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			drain, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			if err := flush(drain); err != nil {
				log.ErrorContext(drain, "outbox: drain on shutdown", "service", service, "err", err)
			}
			cancel()
			return
		case <-ticker.C:
			flushWithRetry(ctx, log, service, flush)
		}
	}
}

// Equal jitter (sleep in [delay/2, delay]) spreads load across replicas better than full jitter
// when many instances tick on the same wall-clock second.
func flushWithRetry(ctx context.Context, log *slog.Logger, service string, flush func(context.Context) error) {
	const (
		maxAttempts = 4
		baseDelay   = 100 * time.Millisecond
	)
	delay := baseDelay
	for attempt := range maxAttempts {
		err := flush(ctx)
		if err == nil {
			return
		}
		if attempt == maxAttempts-1 {
			log.ErrorContext(ctx, "outbox: flush failed after retries", "service", service, "attempts", maxAttempts, "err", err)
			return
		}
		sleep := delay/2 + time.Duration(rand.Int63n(int64(delay)/2+1))
		log.WarnContext(ctx, "outbox: flush error, retrying", "service", service, "attempt", attempt+1, "sleep", sleep, "err", err)
		select {
		case <-time.After(sleep):
		case <-ctx.Done():
			return
		}
		delay *= 2
	}
}

