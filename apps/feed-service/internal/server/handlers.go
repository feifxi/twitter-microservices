package server

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/twitter/shared/auth"
	"github.com/twitter/shared/httperr"
)

const (
	defaultFeedLimit = 20
	maxFeedLimit     = 100
	defaultTrendingLimit = 10
	maxTrendingLimit     = 50
)

func (s *Server) handleFollowingFeed(c *gin.Context) {
	claims := auth.MustClaimsFrom(c)

	limit := defaultFeedLimit
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= maxFeedLimit {
			limit = n
		}
	}
	var cursor *string
	if raw := c.Query("cursor"); raw != "" {
		cursor = &raw
	}

	items, nextCursor, err := s.feed.GetFollowingFeed(c.Request.Context(), claims.Sub, cursor, limit)
	if err != nil {
		httperr.Handle(c, err, s.log)
		return
	}
	c.JSON(http.StatusOK, feedResponse{Items: newFeedItems(items), NextCursor: nextCursor})
}

func (s *Server) handleRecommendedFeed(c *gin.Context) {
	claims := auth.MustClaimsFrom(c)

	limit := defaultFeedLimit
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= maxFeedLimit {
			limit = n
		}
	}
	var cursor *string
	if raw := c.Query("cursor"); raw != "" {
		cursor = &raw
	}

	items, nextCursor, err := s.feed.GetRecommendedFeed(c.Request.Context(), claims.Sub, cursor, limit)
	if err != nil {
		httperr.Handle(c, err, s.log)
		return
	}
	c.JSON(http.StatusOK, feedResponse{Items: newFeedItems(items), NextCursor: nextCursor})
}

func (s *Server) handleTrending(c *gin.Context) {
	limit := defaultTrendingLimit
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && n <= maxTrendingLimit {
			limit = n
		}
	}

	tags, err := s.feed.GetTrending(c.Request.Context(), limit)
	if err != nil {
		httperr.Handle(c, err, s.log)
		return
	}
	c.JSON(http.StatusOK, trendingResponse{Trending: tags})
}
