package server

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/twitter/shared/auth"
	"github.com/twitter/shared/httperr"
	"github.com/twitter/user-service/internal/user"
)

func (s *Server) handleGetUser(c *gin.Context) {
	claims := auth.MustClaimsFrom(c)
	profile, err := s.user.GetProfile(c.Request.Context(), c.Param("id"), claims.Sub)
	if err != nil {
		httperr.Handle(c, err, s.log)
		return
	}

	c.JSON(http.StatusOK, newUserResponseFromProfile(profile, claims.Sub))
}

func (s *Server) handleGetMe(c *gin.Context) {
	claims := auth.MustClaimsFrom(c)
	profile, err := s.user.GetProfile(c.Request.Context(), claims.Sub, claims.Sub)
	if err != nil {
		httperr.Handle(c, err, s.log)
		return
	}
	c.JSON(http.StatusOK, newUserResponseFromProfile(profile, claims.Sub))
}

func (s *Server) handleUpdateMe(c *gin.Context) {
	claims := auth.MustClaimsFrom(c)
	var req updateProfileRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httperr.Validation(c, err)
		return
	}
	u, err := s.user.UpdateProfile(c.Request.Context(), claims.Sub, user.UpdateProfileParams{
		Username:       req.Username,
		DisplayName:    req.DisplayName,
		AvatarURL:      req.AvatarURL,
		HeaderImageURL: req.HeaderImageURL,
		Bio:            req.Bio,
		WebsiteURL:     req.WebsiteURL,
		Location:       req.Location,
	})
	if err != nil {
		httperr.Handle(c, err, s.log)
		return
	}
	c.JSON(http.StatusOK, newUserResponse(u))
}

func (s *Server) handleFollow(c *gin.Context) {
	claims := auth.MustClaimsFrom(c)
	if err := s.user.Follow(c.Request.Context(), claims.Sub, c.Param("id")); err != nil {
		httperr.Handle(c, err, s.log)
		return
	}
	c.JSON(http.StatusOK, followResponse{Following: true})
}

func (s *Server) handleUnfollow(c *gin.Context) {
	claims := auth.MustClaimsFrom(c)
	if err := s.user.Unfollow(c.Request.Context(), claims.Sub, c.Param("id")); err != nil {
		httperr.Handle(c, err, s.log)
		return
	}
	c.JSON(http.StatusOK, followResponse{Following: false})
}

func (s *Server) handleListFollowers(c *gin.Context) {
	claims := auth.MustClaimsFrom(c)
	items, next, err := s.user.ListFollowers(
		c.Request.Context(),
		c.Param("id"),
		claims.Sub,
		queryCursor(c),
		queryLimit(c),
	)
	if err != nil {
		httperr.Handle(c, err, s.log)
		return
	}
	c.JSON(http.StatusOK, newUserListResponse(items, next))
}

func (s *Server) handleListFollowing(c *gin.Context) {
	claims := auth.MustClaimsFrom(c)
	items, next, err := s.user.ListFollowing(
		c.Request.Context(),
		c.Param("id"),
		claims.Sub,
		queryCursor(c),
		queryLimit(c),
	)
	if err != nil {
		httperr.Handle(c, err, s.log)
		return
	}
	c.JSON(http.StatusOK, newUserListResponse(items, next))
}

func (s *Server) handleListSuggestions(c *gin.Context) {
	claims := auth.MustClaimsFrom(c)
	items, err := s.user.ListSuggestions(c.Request.Context(), claims.Sub, queryLimit(c))
	if err != nil {
		httperr.Handle(c, err, s.log)
		return
	}
	c.JSON(http.StatusOK, newUserListResponse(items, nil))
}

func newUserListResponse(items []user.UserListItem, next *string) userListResponse {
	users := make([]userListItem, len(items))
	for i, it := range items {
		users[i] = newUserListItem(it)
	}
	return userListResponse{Users: users, NextCursor: next}
}

func queryCursor(c *gin.Context) *string {
	if raw := c.Query("cursor"); raw != "" {
		return &raw
	}
	return nil
}

func queryLimit(c *gin.Context) int {
	raw := c.Query("limit")
	if raw == "" {
		return 0
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n < 0 {
		return 0
	}
	return n
}
