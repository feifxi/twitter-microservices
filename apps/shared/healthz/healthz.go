package healthz

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
)

const probeTimeout = 1500 * time.Millisecond

type Checker interface {
	Name() string
	Check(ctx context.Context) error
}

// Func wraps any ping-style function into a Checker so the shared package
// doesn't pull in driver-specific dependencies (pgxpool, redis, opensearch…).
// Each service builds its own list of Checkers in main.go.
func Func(name string, fn func(ctx context.Context) error) Checker {
	return funcChecker{name: name, fn: fn}
}

type funcChecker struct {
	name string
	fn   func(ctx context.Context) error
}

func (f funcChecker) Name() string                       { return f.name }
func (f funcChecker) Check(ctx context.Context) error    { return f.fn(ctx) }

type depStatus struct {
	Name   string `json:"name"`
	Status string `json:"status"`
	Error  string `json:"error"`
}

type response struct {
	Status  string      `json:"status"`
	Service string      `json:"service"`
	Deps    []depStatus `json:"deps"`
}

// Handler runs all checkers concurrently with a shared timeout. Returns 200 if
// every dep is reachable, 503 with a per-dep status body if any fails.
// Suitable for k8s readinessProbe and load-balancer health checks.
func Handler(service string, deps ...Checker) gin.HandlerFunc {
	return func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), probeTimeout)
		defer cancel()

		results := make([]depStatus, len(deps))
		var wg sync.WaitGroup
		for i, d := range deps {
			wg.Go(func() {
				if err := d.Check(ctx); err != nil {
					results[i] = depStatus{Name: d.Name(), Status: "error", Error: err.Error()}
					return
				}
				results[i] = depStatus{Name: d.Name(), Status: "ok"}
			})
		}
		wg.Wait()

		status := "ok"
		code := http.StatusOK
		for _, r := range results {
			if r.Status != "ok" {
				status = "degraded"
				code = http.StatusServiceUnavailable
				break
			}
		}
		c.JSON(code, response{Status: status, Service: service, Deps: results})
	}
}

// Livez always returns 200 — the process is up. Used for k8s livenessProbe and
// container-level health checks where a transient dep blip shouldn't restart
// the pod.
func Livez(service string) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.JSON(http.StatusOK, response{Status: "ok", Service: service, Deps: []depStatus{}})
	}
}
