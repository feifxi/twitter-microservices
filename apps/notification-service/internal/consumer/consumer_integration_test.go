//go:build integration

package consumer_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/segmentio/kafka-go"
	"github.com/testcontainers/testcontainers-go/modules/redpanda"
	"github.com/twitter/notification-service/internal/consumer"
	"github.com/twitter/shared/events"
)

// stubHandler is a minimal EventHandler that records calls and can be
// programmed to error a configurable number of times before succeeding.
type stubHandler struct {
	mu              sync.Mutex
	likeCalls       int32
	errorsRemaining int32 // atomic decremented; non-zero means return error
}

func (s *stubHandler) CreateFromLike(ctx context.Context, _ int64, actorID, authorID, tweetID string) error {
	atomic.AddInt32(&s.likeCalls, 1)
	if atomic.AddInt32(&s.errorsRemaining, -1) >= 0 {
		return errors.New("stub: transient failure")
	}
	return nil
}

func (s *stubHandler) CreateFromRetweet(ctx context.Context, _ int64, _, _, _ string) error {
	return nil
}
func (s *stubHandler) DeleteFromRetweet(ctx context.Context, _, _ string) error {
	return nil
}
func (s *stubHandler) CreateFromReply(ctx context.Context, _ int64, _, _, _ string) error {
	return nil
}
func (s *stubHandler) CreateFromFollow(ctx context.Context, _ int64, _, _ string) error {
	return nil
}

// startRedpanda boots a single-node redpanda broker and returns its bootstrap address.
func startRedpanda(t *testing.T) string {
	t.Helper()
	ctx := context.Background()
	ctr, err := redpanda.Run(ctx, "docker.redpanda.com/redpandadata/redpanda:v23.3.11")
	if err != nil {
		t.Fatalf("start redpanda: %v", err)
	}
	t.Cleanup(func() { _ = ctr.Terminate(ctx) })
	brokers, err := ctr.KafkaSeedBroker(ctx)
	if err != nil {
		t.Fatalf("get kafka brokers: %v", err)
	}
	return brokers
}

// Pre-creates a topic so the first WriteMessages doesn't race with kafka-go's
// AllowAutoTopicCreation. Without this, the producer occasionally hits the
// broker before the auto-create finishes and fails with
// "Unknown Topic Or Partition".
func createTopic(t *testing.T, broker, topic string) {
	t.Helper()
	conn, err := kafka.DialContext(context.Background(), "tcp", broker)
	if err != nil {
		t.Fatalf("dial broker: %v", err)
	}
	defer conn.Close()
	if err := conn.CreateTopics(kafka.TopicConfig{
		Topic:             topic,
		NumPartitions:     1,
		ReplicationFactor: 1,
	}); err != nil {
		t.Fatalf("create topic %s: %v", topic, err)
	}
}

// TestConsumer_RedeliversOnDispatchError is the red→green test for PR 1.
//
// Scenario: publish one tweet.liked event. The stub handler errors on the
// first call (simulating a transient DB outage). The consumer must redeliver
// the message so the second call succeeds — i.e. handler must be invoked
// EXACTLY TWICE.
//
// Under the old code (ReadMessage auto-commits before dispatch), the offset
// advances after the first failing call and the message is never redelivered;
// the handler is called only once and this test times out → RED.
//
// Under the new code (FetchMessage + CommitMessages on success only), the
// failed dispatch leaves the offset uncommitted; on the next Fetch the broker
// redelivers the same message and the second call succeeds → GREEN.
func TestConsumer_RedeliversOnDispatchError(t *testing.T) {
	broker := startRedpanda(t)

	const topic = events.TopicTweetLiked
	const groupID = "notification-service-cg-test"

	createTopic(t, broker, topic)

	// Publish one tweet.liked event.
	w := &kafka.Writer{
		Addr:     kafka.TCP(broker),
		Topic:    topic,
		Balancer: &kafka.Hash{},
	}
	defer w.Close()

	evt := events.TweetLikedEvent{
		TweetID:  "tw_test01",
		LikerID:  "usr_liker",
		AuthorID: "usr_author",
	}
	payload, _ := json.Marshal(evt)
	if err := w.WriteMessages(context.Background(), kafka.Message{Key: []byte(evt.TweetID), Value: payload}); err != nil {
		t.Fatalf("publish: %v", err)
	}

	// Reader bound to a fresh consumer group so we read from the earliest offset.
	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:     []string{broker},
		Topic:       topic,
		GroupID:     groupID,
		StartOffset: kafka.FirstOffset,
	})
	defer r.Close()

	// Stub that errors on the first call, then succeeds.
	stub := &stubHandler{errorsRemaining: 1}
	log := slog.New(slog.NewTextHandler(io.Discard, &slog.HandlerOptions{Level: slog.LevelDebug}))
	c := consumer.New(stub, nil /* dedup unused in test */, nil /* rdb unused for tweet.liked */, nil /* dlq nil in tests */, log)

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	go c.Run(ctx, r)

	// Wait until the handler has been called twice, or fail on timeout.
	deadline := time.After(25 * time.Second)
	for {
		if atomic.LoadInt32(&stub.likeCalls) >= 2 {
			cancel()
			return // GREEN
		}
		select {
		case <-deadline:
			t.Fatalf("handler called %d times; expected 2 (message was not redelivered after dispatch error)", atomic.LoadInt32(&stub.likeCalls))
		case <-time.After(100 * time.Millisecond):
		}
	}
}
