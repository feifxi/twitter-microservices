package dlq

import (
	"context"
	"log/slog"
	"strconv"

	"github.com/segmentio/kafka-go"
)

const (
	HeaderSourceTopic     = "X-DLQ-Source-Topic"
	HeaderSourcePartition = "X-DLQ-Source-Partition"
	HeaderSourceOffset    = "X-DLQ-Source-Offset"
	HeaderError           = "X-DLQ-Error"
)

// No-op when w is nil (test environments).
func Write(ctx context.Context, w *kafka.Writer, log *slog.Logger, msg kafka.Message, reason string) {
	if w == nil {
		return
	}
	headers := make([]kafka.Header, 0, len(msg.Headers)+4)
	headers = append(headers, msg.Headers...)
	headers = append(headers,
		kafka.Header{Key: HeaderSourceTopic, Value: []byte(msg.Topic)},
		kafka.Header{Key: HeaderSourcePartition, Value: []byte(strconv.Itoa(msg.Partition))},
		kafka.Header{Key: HeaderSourceOffset, Value: []byte(strconv.FormatInt(msg.Offset, 10))},
		kafka.Header{Key: HeaderError, Value: []byte(reason)},
	)
	if err := w.WriteMessages(ctx, kafka.Message{
		Key:     msg.Key,
		Value:   msg.Value,
		Headers: headers,
	}); err != nil {
		log.ErrorContext(ctx, "dlq: write failed", "source_topic", msg.Topic, "offset", msg.Offset, "err", err)
	}
}
