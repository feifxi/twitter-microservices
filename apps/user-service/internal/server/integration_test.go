//go:build integration

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/twitter/shared/dbmigrate"
	db "github.com/twitter/user-service/db/sqlc"
	"github.com/twitter/user-service/internal/server"
	"github.com/twitter/user-service/internal/user"
)

const serviceToken = "test-service-token"

// stubKeycloak satisfies keycloak.KeycloakAdmin without a real Keycloak instance.
type stubKeycloak struct{}

func (stubKeycloak) UpdateUsername(_ context.Context, _, _ string) error { return nil }

func testServer(t *testing.T) *httptest.Server {
	t.Helper()
	ctx := context.Background()

	pgc, err := postgres.Run(ctx,
		"postgres:16-alpine",
		postgres.WithDatabase("testdb"),
		postgres.WithUsername("test"),
		postgres.WithPassword("test"),
		postgres.BasicWaitStrategies(),
	)
	if err != nil {
		t.Fatalf("start postgres: %v", err)
	}
	t.Cleanup(func() { pgc.Terminate(ctx) })

	migrateURL, err := pgc.ConnectionString(ctx, "sslmode=disable", "x-migrations-table=user_migrations")
	if err != nil {
		t.Fatalf("get migrate url: %v", err)
	}
	if err := dbmigrate.RunSource("file://../../migrations", migrateURL); err != nil {
		t.Fatalf("run migrations: %v", err)
	}

	poolURL, err := pgc.ConnectionString(ctx, "sslmode=disable", "search_path=users")
	if err != nil {
		t.Fatalf("get pool url: %v", err)
	}
	pool, err := pgxpool.New(ctx, poolURL)
	if err != nil {
		t.Fatalf("connect db: %v", err)
	}
	t.Cleanup(pool.Close)

	// nil writer: outbox flush guards against nil kb, so no Kafka broker needed in tests.
	userSvc := user.New(db.NewStore(pool), nil, stubKeycloak{}, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	srv := server.New(userSvc, serviceToken, slog.New(slog.NewTextHandler(os.Stderr, nil)))

	return httptest.NewServer(srv.Handler())
}

// ── Helpers ───────────────────────────────────────────────────────────────────

func postJSON(t *testing.T, url string, body any, headers map[string]string) *http.Response {
	t.Helper()
	b, _ := json.Marshal(body)
	req, _ := http.NewRequest(http.MethodPost, url, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	for k, v := range headers {
		req.Header.Set(k, v)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	return resp
}

func decodeJSON(t *testing.T, resp *http.Response, dst any) {
	t.Helper()
	defer resp.Body.Close()
	if err := json.NewDecoder(resp.Body).Decode(dst); err != nil {
		t.Fatalf("decode response: %v", err)
	}
}

func provisionHeaders() map[string]string {
	return map[string]string{"Authorization": "Bearer " + serviceToken}
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestIntegration_Provision(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	// Simulate Keycloak SPI calling /internal/provision with a sub UUID.
	resp := postJSON(t, srv.URL+"/internal/provision", map[string]any{
		"keycloak_sub": "550e8400-e29b-41d4-a716-446655440001",
		"email":        "alice@example.com",
		"display_name": "Alice",
	}, provisionHeaders())
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var body map[string]any
	decodeJSON(t, resp, &body)
	if body["id"] != "550e8400-e29b-41d4-a716-446655440001" {
		t.Fatalf("expected Keycloak sub as id, got %v", body["id"])
	}
	if body["username"] != nil {
		t.Fatalf("expected null username before onboarding, got %v", body["username"])
	}
}

func TestIntegration_Provision_Idempotent(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	payload := map[string]any{
		"keycloak_sub": "550e8400-e29b-41d4-a716-446655440002",
		"email":        "bob@example.com",
		"display_name": "Bob",
	}
	postJSON(t, srv.URL+"/internal/provision", payload, provisionHeaders()).Body.Close()

	// Second call for the same sub must return 200 (idempotent).
	resp := postJSON(t, srv.URL+"/internal/provision", payload, provisionHeaders())
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200 on duplicate provision, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestIntegration_Provision_MissingToken(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	resp := postJSON(t, srv.URL+"/internal/provision", map[string]any{
		"keycloak_sub": "550e8400-e29b-41d4-a716-446655440003",
		"email":        "carol@example.com",
	}, nil)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401 without service token, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}

func TestIntegration_GetUser(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	sub := "550e8400-e29b-41d4-a716-446655440010"
	postJSON(t, srv.URL+"/internal/provision", map[string]any{
		"keycloak_sub": sub,
		"email":        "dave@example.com",
		"display_name": "Dave",
	}, provisionHeaders()).Body.Close()

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/v1/users/"+sub, nil)
	req.Header.Set("X-User-ID", sub)
	req.Header.Set("X-User-Email", "dave@example.com")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET user: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]any
	decodeJSON(t, resp, &body)
	if body["id"] != sub {
		t.Fatalf("expected id %s, got %v", sub, body["id"])
	}
}

func TestIntegration_UpdateProfile_SetsUsername(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	sub := "550e8400-e29b-41d4-a716-446655440011"
	postJSON(t, srv.URL+"/internal/provision", map[string]any{
		"keycloak_sub": sub, "email": "eve@example.com", "display_name": "Eve",
	}, provisionHeaders()).Body.Close()

	b, _ := json.Marshal(map[string]any{"username": "eve_handle"})
	req, _ := http.NewRequest(http.MethodPatch, srv.URL+"/v1/users/"+sub, bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", sub)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("PATCH user: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	var body map[string]any
	decodeJSON(t, resp, &body)
	if body["username"] != "eve_handle" {
		t.Fatalf("expected username eve_handle, got %v", body["username"])
	}
}

func TestIntegration_Follow_Unfollow(t *testing.T) {
	srv := testServer(t)
	defer srv.Close()

	aliceSub := "550e8400-e29b-41d4-a716-446655440020"
	bobSub := "550e8400-e29b-41d4-a716-446655440021"

	for _, u := range []struct{ sub, email, name string }{
		{aliceSub, "alice3@example.com", "Alice"},
		{bobSub, "bob3@example.com", "Bob"},
	} {
		postJSON(t, srv.URL+"/internal/provision", map[string]any{
			"keycloak_sub": u.sub, "email": u.email, "display_name": u.name,
		}, provisionHeaders()).Body.Close()
	}

	followURL := fmt.Sprintf("%s/v1/users/%s/follow", srv.URL, bobSub)
	req, _ := http.NewRequest(http.MethodPost, followURL, nil)
	req.Header.Set("X-User-ID", aliceSub)
	resp, _ := http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("follow: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()

	req, _ = http.NewRequest(http.MethodDelete, followURL, nil)
	req.Header.Set("X-User-ID", aliceSub)
	resp, _ = http.DefaultClient.Do(req)
	if resp.StatusCode != http.StatusOK {
		resp.Body.Close()
		t.Fatalf("unfollow: expected 200, got %d", resp.StatusCode)
	}
	resp.Body.Close()
}
