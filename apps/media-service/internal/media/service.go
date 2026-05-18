package media

import (
	"net/http"
	"context"
	"time"

	v4 "github.com/aws/aws-sdk-go-v2/aws/signer/v4"
	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	"github.com/oklog/ulid/v2"
	"github.com/twitter/shared/httperr"
)

var allowedContentTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/gif":  true,
	"image/webp": true,
	"video/mp4":  true,
}

type presignAPI interface {
	PresignPutObject(ctx context.Context, params *s3.PutObjectInput, optFns ...func(*s3.PresignOptions)) (*v4.PresignedHTTPRequest, error)
}

type PresignResult struct {
	MediaID   string
	UploadURL string
	PublicURL string
	ExpiresAt time.Time
}

type Service struct {
	s3presign     presignAPI
	bucket        string
	publicURLBase string
}

func New(s3presign presignAPI, bucket, publicURLBase string) *Service {
	return &Service{s3presign: s3presign, bucket: bucket, publicURLBase: publicURLBase}
}

const presignTTL = 15 * time.Minute

func (s *Service) Presign(ctx context.Context, userID, contentType string) (*PresignResult, error) {
	if !allowedContentTypes[contentType] {
		return nil, httperr.New(http.StatusUnprocessableEntity, "UNSUPPORTED_CONTENT_TYPE", "content type not allowed; use image/jpeg, image/png, image/gif, image/webp, or video/mp4")
	}

	mediaID := "med_" + ulid.Make().String()
	s3Key := "uploads/" + userID + "/" + mediaID

	presigned, err := s.s3presign.PresignPutObject(ctx, &s3.PutObjectInput{
		Bucket:      aws.String(s.bucket),
		Key:         aws.String(s3Key),
		ContentType: aws.String(contentType),
	}, s3.WithPresignExpires(presignTTL))
	if err != nil {
		return nil, err
	}

	return &PresignResult{
		MediaID:   mediaID,
		UploadURL: presigned.URL,
		PublicURL: s.publicURLBase + "/" + s3Key,
		ExpiresAt: time.Now().Add(presignTTL).UTC(),
	}, nil
}
