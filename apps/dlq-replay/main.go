// Command dlq-replay reads messages from a dead-letter queue topic and
// republishes each one to its original source topic.
//
// Each DLQ message carries provenance headers written by writeToDLQ:
//
//	X-DLQ-Source-Topic     — original Kafka topic
//	X-DLQ-Source-Partition — original partition (informational)
//	X-DLQ-Source-Offset    — original offset (informational)
//	X-DLQ-Error            — reason the message was sent to the DLQ
//
// Usage:
//
//	dlq-replay -brokers localhost:9092 -topic notification-service-cg.dlq
//	dlq-replay -brokers localhost:9092 -topic feed-service-cg.dlq -group dlq-replay-feed
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/segmentio/kafka-go"
	"github.com/twitter/shared/dlq"
)

func main() {
	brokers := flag.String("brokers", "localhost:9092", "comma-separated Kafka broker addresses")
	topic := flag.String("topic", "", "DLQ topic to replay from (required)")
	groupID := flag.String("group", "dlq-replay", "consumer group ID for offset tracking")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))

	if *topic == "" {
		log.Error("-topic is required")
		os.Exit(1)
	}

	brokerList := strings.Split(*brokers, ",")

	r := kafka.NewReader(kafka.ReaderConfig{
		Brokers:  brokerList,
		GroupID:  *groupID,
		Topic:    *topic,
		MinBytes: 1,
		MaxBytes: 1e6,
	})
	defer r.Close()

	w := &kafka.Writer{
		Addr:         kafka.TCP(brokerList...),
		Balancer:     &kafka.LeastBytes{},
		RequiredAcks: kafka.RequireOne,
	}
	defer w.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	log.Info("starting dlq replay", "topic", *topic, "group", *groupID)
	replayed := 0
	for {
		msg, err := r.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				break
			}
			log.Error("fetch", "err", err)
			continue
		}

		sourceTopic := headerValue(msg.Headers, dlq.HeaderSourceTopic)
		if sourceTopic == "" {
			log.Warn("message missing "+dlq.HeaderSourceTopic+"; skipping", "offset", msg.Offset)
			if err := r.CommitMessages(ctx, msg); err != nil {
				log.Error("commit (skip)", "err", err)
			}
			continue
		}

		// Strip DLQ provenance headers before replaying to the original topic.
		replay := kafka.Message{
			Topic:   sourceTopic,
			Key:     msg.Key,
			Value:   msg.Value,
			Headers: stripDLQHeaders(msg.Headers),
		}
		if err := w.WriteMessages(ctx, replay); err != nil {
			log.Error("replay write failed; will retry", "source_topic", sourceTopic, "offset", msg.Offset, "err", err)
			continue
		}
		if err := r.CommitMessages(ctx, msg); err != nil {
			log.Error("commit", "err", err)
		}
		replayed++
		log.Info("replayed", "n", replayed, "source_topic", sourceTopic,
			"offset", msg.Offset,
			"dlq_error", headerValue(msg.Headers, dlq.HeaderError),
		)
	}

	log.Info("done", "replayed", replayed)
}

func headerValue(headers []kafka.Header, key string) string {
	for _, h := range headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

var dlqHeaders = map[string]bool{
	dlq.HeaderSourceTopic:     true,
	dlq.HeaderSourcePartition: true,
	dlq.HeaderSourceOffset:    true,
	dlq.HeaderError:           true,
}

func stripDLQHeaders(headers []kafka.Header) []kafka.Header {
	result := make([]kafka.Header, 0, len(headers))
	for _, h := range headers {
		if !dlqHeaders[h.Key] {
			result = append(result, h)
		}
	}
	return result
}
