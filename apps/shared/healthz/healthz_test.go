package healthz_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/twitter/shared/healthz"
)

func TestLivez_AlwaysOK(t *testing.T) {
	w := callRoute("/livez", healthz.Livez("svc"))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] != "ok" || body["service"] != "svc" {
		t.Errorf("body = %v, want status=ok service=svc", body)
	}
	if deps, _ := body["deps"].([]any); len(deps) != 0 {
		t.Errorf("deps = %v, want empty slice", deps)
	}
}

func TestHandler_AllOK(t *testing.T) {
	pg := healthz.Func("postgres", func(context.Context) error { return nil })
	rd := healthz.Func("redis", func(context.Context) error { return nil })
	w := callRoute("/healthz", healthz.Handler("svc", pg, rd))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
}

func TestHandler_OneDown_Returns503(t *testing.T) {
	pg := healthz.Func("postgres", func(context.Context) error { return nil })
	rd := healthz.Func("redis", func(context.Context) error { return errors.New("connection refused") })
	w := callRoute("/healthz", healthz.Handler("svc", pg, rd))
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503", w.Code)
	}
	var body struct {
		Status string `json:"status"`
		Deps   []struct {
			Name   string `json:"name"`
			Status string `json:"status"`
			Error  string `json:"error"`
		} `json:"deps"`
	}
	json.Unmarshal(w.Body.Bytes(), &body)
	if body.Status != "degraded" {
		t.Errorf("status = %q, want degraded", body.Status)
	}
	var rdEntry *struct {
		Name   string `json:"name"`
		Status string `json:"status"`
		Error  string `json:"error"`
	}
	for i := range body.Deps {
		if body.Deps[i].Name == "redis" {
			rdEntry = &body.Deps[i]
		}
	}
	if rdEntry == nil || rdEntry.Status != "error" || rdEntry.Error == "" {
		t.Errorf("redis dep entry = %+v, want status=error with non-empty error", rdEntry)
	}
}

func TestHandler_RunsProbesConcurrently(t *testing.T) {
	const sleep = 100 * time.Millisecond
	slowA := healthz.Func("a", func(ctx context.Context) error {
		select {
		case <-time.After(sleep):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	slowB := healthz.Func("b", func(ctx context.Context) error {
		select {
		case <-time.After(sleep):
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	})
	start := time.Now()
	w := callRoute("/healthz", healthz.Handler("svc", slowA, slowB))
	elapsed := time.Since(start)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}
	// Sequential would be ≥2×sleep. Concurrent stays close to 1×sleep.
	if elapsed > sleep*2 {
		t.Errorf("elapsed = %s, expected close to %s (concurrent)", elapsed, sleep)
	}
}

func TestHandler_NoDeps_StillReturnsOK(t *testing.T) {
	w := callRoute("/healthz", healthz.Handler("svc"))
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 with no deps", w.Code)
	}
}

func TestFunc_PassesContext(t *testing.T) {
	var saw atomic.Bool
	c := healthz.Func("ctx-aware", func(ctx context.Context) error {
		if _, ok := ctx.Deadline(); ok {
			saw.Store(true)
		}
		return nil
	})
	callRoute("/healthz", healthz.Handler("svc", c))
	if !saw.Load() {
		t.Error("checker did not receive a deadline-bearing context")
	}
}

func callRoute(path string, h gin.HandlerFunc) *httptest.ResponseRecorder {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.GET(path, h)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, path, nil)
	r.ServeHTTP(w, req)
	return w
}
