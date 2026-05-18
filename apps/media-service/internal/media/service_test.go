package media_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/twitter/media-service/internal/media"
	"github.com/twitter/shared/httperr"
)

// stubPresign implements the presignAPI interface used by media.Service.
type stubPresign struct {
	url string
	err error
}

func (s *stubPresign) PresignPutObject(_ context.Context, input *s3.PutObjectInput, _ ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &v4.PresignedHTTPRequest{URL: s.url}, nil
}

func newService(p *stubPresign) *media.Service {
	return media.New(p, "my-bucket", "https://cdn.example.com")
}

// ── content-type validation ───────────────────────────────────────────────────

func TestPresign_UnsupportedContentTypeReturnsError(t *testing.T) {
	svc := newService(&stubPresign{url: "https://s3/upload"})
	_, err := svc.Presign(context.Background(), "usr_A", "application/pdf")
	var e *httperr.AppError
	if !errors.As(err, &e) || e.Code != "UNSUPPORTED_CONTENT_TYPE" {
		t.Errorf("Presign(pdf) = %v, want UNSUPPORTED_CONTENT_TYPE httperr", err)
	}
}

func TestPresign_AllAllowedContentTypesSucceed(t *testing.T) {
	allowed := []string{"image/jpeg", "image/png", "image/gif", "image/webp", "video/mp4"}
	svc := newService(&stubPresign{url: "https://s3/upload"})
	for _, ct := range allowed {
		if _, err := svc.Presign(context.Background(), "usr_A", ct); err != nil {
			t.Errorf("Presign(%q) = %v, want nil", ct, err)
		}
	}
}

// ── presign result shape ──────────────────────────────────────────────────────

func TestPresign_ResultContainsExpectedFields(t *testing.T) {
	svc := newService(&stubPresign{url: "https://s3.amazonaws.com/my-bucket/uploads/usr_A/med_123?sig=x"})
	result, err := svc.Presign(context.Background(), "usr_A", "image/jpeg")
	if err != nil {
		t.Fatalf("Presign: %v", err)
	}

	if !strings.HasPrefix(result.MediaID, "med_") {
		t.Errorf("media_id = %q, want med_ prefix", result.MediaID)
	}
	if result.UploadURL == "" {
		t.Error("upload_url should not be empty")
	}
	if !strings.HasPrefix(result.PublicURL, "https://cdn.example.com/uploads/usr_A/") {
		t.Errorf("public_url = %q, want https://cdn.example.com/uploads/usr_A/... prefix", result.PublicURL)
	}
	if result.ExpiresAt.Before(time.Now()) {
		t.Error("expires_at should be in the future")
	}
}

func TestPresign_S3ErrorPropagated(t *testing.T) {
	s3Err := errors.New("s3 unavailable")
	svc := newService(&stubPresign{err: s3Err})
	_, err := svc.Presign(context.Background(), "usr_A", "image/png")
	if !errors.Is(err, s3Err) {
		t.Errorf("Presign(s3 error) = %v, want s3 error", err)
	}
}
