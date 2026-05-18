package metrics

import (
	"context"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"google.golang.org/grpc"
	"google.golang.org/grpc/status"
)

var (
	httpRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "http_requests_total",
		Help: "Total HTTP requests by method, path, and status code.",
	}, []string{"service", "method", "path", "status"})

	httpDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_request_duration_seconds",
		Help:    "HTTP request latency by method and path.",
		Buckets: prometheus.DefBuckets,
	}, []string{"service", "method", "path"})

	KafkaConsumerErrors = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "kafka_consumer_errors_total",
		Help: "Total Kafka consumer dispatch errors by service and topic.",
	}, []string{"service", "topic"})

	OutboxDepth = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "outbox_pending_rows",
		Help: "Number of unsent rows in the transactional outbox.",
	}, []string{"service"})

	grpcRequests = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "grpc_server_requests_total",
		Help: "Total gRPC server requests by service, method, and code.",
	}, []string{"service", "method", "code"})

	grpcDuration = promauto.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "grpc_server_duration_seconds",
		Help:    "gRPC server handler latency by method.",
		Buckets: prometheus.DefBuckets,
	}, []string{"service", "method"})
)

func GinMiddleware(serviceName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()

		path := c.FullPath()
		if path == "" {
			path = "unknown"
		}
		code := strconv.Itoa(c.Writer.Status())
		httpRequests.WithLabelValues(serviceName, c.Request.Method, path, code).Inc()
		httpDuration.WithLabelValues(serviceName, c.Request.Method, path).Observe(time.Since(start).Seconds())
	}
}

func MetricsHandler() gin.HandlerFunc {
	h := promhttp.Handler()
	return func(c *gin.Context) {
		h.ServeHTTP(c.Writer, c.Request)
	}
}

// Status code labels use gRPC canonical names ("OK", "NotFound", …) to align with otelgrpc traces.
func GRPCServerInterceptor(serviceName string) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
		start := time.Now()
		resp, err := handler(ctx, req)
		code := status.Code(err).String()
		grpcRequests.WithLabelValues(serviceName, info.FullMethod, code).Inc()
		grpcDuration.WithLabelValues(serviceName, info.FullMethod).Observe(time.Since(start).Seconds())
		return resp, err
	}
}
