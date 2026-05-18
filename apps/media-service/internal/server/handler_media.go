package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/twitter/shared/auth"
	"github.com/twitter/shared/httperr"
)

func (s *Server) handlePresign(c *gin.Context) {
	claims := auth.MustClaimsFrom(c)

	var req presignRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httperr.Validation(c, err)
		return
	}

	result, err := s.media.Presign(c.Request.Context(), claims.Sub, req.ContentType)
	if err != nil {
		httperr.Handle(c, err, s.log)
		return
	}

	c.JSON(http.StatusOK, presignResponse{
		MediaID:   result.MediaID,
		UploadURL: result.UploadURL,
		PublicURL: result.PublicURL,
		ExpiresAt: result.ExpiresAt,
	})
}
