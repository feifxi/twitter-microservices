package server

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/twitter/shared/auth"
	"github.com/twitter/shared/httperr"
)

const (
	defaultSearchLimit = 20
	maxSearchLimit     = 100
)

func (s *Server) handleSearchTweets(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		c.JSON(http.StatusOK, tweetSearchResponse{Tweets: []tweetResult{}, NextCursor: nil})
		return
	}

	mode := c.Query("mode")
	limit := parseLimit(c.Query("limit"))
	var cursor *string
	if raw := c.Query("cursor"); raw != "" {
		cursor = &raw
	}

	claims := auth.MustClaimsFrom(c)

	enriched, nextCursor, effectiveMode, err := s.search.SearchTweets(c.Request.Context(), q, mode, claims.Sub, cursor, limit)
	if err != nil {
		httperr.Handle(c, err, s.log)
		return
	}

	if effectiveMode != mode {
		c.Header("X-Search-Mode", "keyword-fallback")
	}

	results := make([]tweetResult, len(enriched))
	for i, r := range enriched {
		results[i] = newTweetResult(r)
	}
	c.JSON(http.StatusOK, tweetSearchResponse{Tweets: results, NextCursor: nextCursor})
}

func (s *Server) handleSearchUsers(c *gin.Context) {
	q := c.Query("q")
	if q == "" {
		c.JSON(http.StatusOK, userSearchResponse{Users: []userResult{}, NextCursor: nil})
		return
	}

	limit := parseLimit(c.Query("limit"))
	var cursor *string
	if raw := c.Query("cursor"); raw != "" {
		cursor = &raw
	}

	claims := auth.MustClaimsFrom(c)

	enriched, nextCursor, err := s.search.SearchUsers(c.Request.Context(), q, claims.Sub, cursor, limit)
	if err != nil {
		httperr.Handle(c, err, s.log)
		return
	}

	results := make([]userResult, len(enriched))
	for i, r := range enriched {
		results[i] = newUserResult(r)
	}
	c.JSON(http.StatusOK, userSearchResponse{Users: results, NextCursor: nextCursor})
}

func parseLimit(raw string) int {
	if raw == "" {
		return defaultSearchLimit
	}
	n, err := strconv.Atoi(raw)
	if err != nil || n <= 0 || n > maxSearchLimit {
		return defaultSearchLimit
	}
	return n
}
