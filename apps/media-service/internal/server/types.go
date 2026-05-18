package server

import "time"

type presignRequest struct {
	ContentType string `json:"content_type" binding:"required"`
}

type presignResponse struct {
	MediaID   string    `json:"media_id"`
	UploadURL string    `json:"upload_url"`
	PublicURL string    `json:"public_url"`
	ExpiresAt time.Time `json:"expires_at"`
}
