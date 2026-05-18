package server

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/twitter/shared/httperr"
)

func (s *Server) handleProvision(c *gin.Context) {
	var req provisionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httperr.Validation(c, err)
		return
	}
	u, created, err := s.user.Provision(c.Request.Context(), req.KeycloakSub, req.Email, req.DisplayName)
	if err != nil {
		httperr.Handle(c, err, s.log)
		return
	}
	status := http.StatusOK
	if created {
		status = http.StatusCreated
	}
	c.JSON(status, newUserResponse(u))
}
