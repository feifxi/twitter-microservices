//go:build integration

package server_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/twitter/media-service/internal/media"
	"github.com/twitter/media-service/internal/server"
)

type stubPresign struct{}

func (stubPresign) PresignPutObject(_ context.Context, in *s3.PutObjectInput, _ ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error) {
	return &v4.PresignedHTTPRequest{
		URL: "https://localstack.local/bucket/" + *in.Key + "?X-Amz-Signature=stub",
	}, nil
}

func newMediaTestServer(t *testing.T) *httptest.Server {
	t.Helper()
	mediaSvc := media.New(stubPresign{}, "twitter-media", "https://media.example.com")
	srv := server.New(mediaSvc, slog.New(slog.NewTextHandler(os.Stderr, nil)))
	return httptest.NewServer(srv.Handler())
}

func presignReq(t *testing.T, url, contentType string) *http.Response {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"content_type": contentType})
	req, _ := http.NewRequest(http.MethodPost, url+"/v1/media/presign", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-User-ID", "usr_mediatest")
	req.Header.Set("X-User-Email", "m@m.com")
	req.Header.Set("X-User-Username", "mediauser")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("presign request: %v", err)
	}
	return resp
}

// ── Tests ─────────────────────────────────────────────────────────────────────

func TestIntegration_Presign_ReturnsAllFields(t *testing.T) {
	ts := newMediaTestServer(t)
	defer ts.Close()

	resp := presignReq(t, ts.URL, "image/jpeg")
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d", resp.StatusCode)
	}
	defer resp.Body.Close()

	var result map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	for _, field := range []string{"media_id", "upload_url", "public_url", "expires_at"} {
		if result[field] == nil {
			t.Errorf("missing field %q in response", field)
		}
	}
	mediaID, _ := result["media_id"].(string)
	if !strings.HasPrefix(mediaID, "med_") {
		t.Errorf("media_id should start with 'med_', got %q", mediaID)
	}
	publicURL, _ := result["public_url"].(string)
	if !strings.HasPrefix(publicURL, "https://media.example.com/") {
		t.Errorf("public_url wrong base: %q", publicURL)
	}
	if !strings.Contains(publicURL, "usr_mediatest") {
		t.Errorf("public_url should contain uploader user_id: %q", publicURL)
	}
}

func TestIntegration_Presign_UnsupportedContentType(t *testing.T) {
	ts := newMediaTestServer(t)
	defer ts.Close()

	resp := presignReq(t, ts.URL, "application/pdf")
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422, got %d", resp.StatusCode)
	}
}

func TestIntegration_Presign_Unauthenticated(t *testing.T) {
	ts := newMediaTestServer(t)
	defer ts.Close()

	body, _ := json.Marshal(map[string]string{"content_type": "image/png"})
	resp, _ := http.Post(ts.URL+"/v1/media/presign", "application/json", bytes.NewReader(body))
	resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", resp.StatusCode)
	}
}
