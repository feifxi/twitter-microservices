package tracing

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"

	"github.com/segmentio/kafka-go"
)

// Written by outbox flushers; read by consumers via the OTel propagator.
const TraceParentHeader = "traceparent"

type kafkaCarrier struct {
	headers *[]kafka.Header
}

func (c kafkaCarrier) Get(key string) string {
	for _, h := range *c.headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}

func (c kafkaCarrier) Set(key, value string) {
	for i, h := range *c.headers {
		if h.Key == key {
			(*c.headers)[i].Value = []byte(value)
			return
		}
	}
	*c.headers = append(*c.headers, kafka.Header{Key: key, Value: []byte(value)})
}

func (c kafkaCarrier) Keys() []string {
	keys := make([]string, len(*c.headers))
	for i, h := range *c.headers {
		keys[i] = h.Key
	}
	return keys
}

// Makes the consumer's span a child of the producer's span.
func FromKafkaMessage(ctx context.Context, msg kafka.Message) context.Context {
	headers := msg.Headers
	return otel.GetTextMapPropagator().Extract(ctx, kafkaCarrier{headers: &headers})
}

func InjectKafkaHeaders(ctx context.Context, headers *[]kafka.Header) {
	otel.GetTextMapPropagator().Inject(ctx, kafkaCarrier{headers: headers})
}

// Returns "" when no span is active.
// Stores the full traceparent (not just trace ID) so consumer spans have a causal link to the
// producer span — storing only the trace ID produces orphan trees with the same trace ID.
func TraceParentFromContext(ctx context.Context) string {
	sc := trace.SpanFromContext(ctx).SpanContext()
	if !sc.IsValid() {
		return ""
	}
	return fmt.Sprintf("00-%s-%s-%02x", sc.TraceID(), sc.SpanID(), byte(sc.TraceFlags()))
}
