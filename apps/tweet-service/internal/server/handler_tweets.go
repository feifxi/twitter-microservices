package server

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/twitter/shared/auth"
	"github.com/twitter/shared/httperr"
	db "github.com/twitter/tweet-service/db/sqlc"
	"github.com/twitter/tweet-service/internal/tweet"
)

const (
	defaultPageLimit = int32(20)
	maxPageLimit     = int32(100)
)

func parseLimit(c *gin.Context) int32 {
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 && int32(n) <= maxPageLimit {
			return int32(n)
		}
	}
	return defaultPageLimit
}

func parseCursor(c *gin.Context) *string {
	if raw := c.Query("cursor"); raw != "" {
		return &raw
	}
	return nil
}

func (s *Server) handleCreateTweet(c *gin.Context) {
	claims := auth.MustClaimsFrom(c)
	var req tweetBodyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httperr.Validation(c, err)
		return
	}
	t, tags, err := s.tweet.Create(c.Request.Context(), claims.Sub, req.Body, req.MediaID, req.MediaURL)
	if err != nil {
		httperr.Handle(c, err, s.log)
		return
	}
	author := s.tweet.FetchAuthor(c.Request.Context(), t.AuthorID)
	c.JSON(http.StatusCreated, newTweetResponse(tweetResponseParams{
		id:           t.ID,
		author:       author,
		body:         t.Body,
		replyToID:    t.ReplyToID,
		mediaID:      t.MediaID,
		mediaURL:     t.MediaUrl,
		likeCount:    t.LikeCount,
		retweetCount: t.RetweetCount,
		replyCount:   t.ReplyCount,
		createdAt:    t.CreatedAt,
		tags:         tags,
	}))
}

func (s *Server) handleGetTweet(c *gin.Context) {
	claims := auth.MustClaimsFrom(c)
	t, err := s.tweet.GetByID(c.Request.Context(), c.Param("id"), claims.Sub)
	if err != nil {
		httperr.Handle(c, err, s.log)
		return
	}
	author := s.tweet.FetchAuthor(c.Request.Context(), t.AuthorID)
	c.JSON(http.StatusOK, newTweetResponse(tweetResponseParams{
		id:           t.ID,
		author:       author,
		body:         t.Body,
		replyToID:    t.ReplyToID,
		mediaID:      t.MediaID,
		mediaURL:     t.MediaUrl,
		likeCount:    t.LikeCount,
		retweetCount: t.RetweetCount,
		replyCount:   t.ReplyCount,
		createdAt:    t.CreatedAt,
		isLiked:      t.IsLiked,
		isRetweeted:  t.IsRetweeted,
	}))
}

func (s *Server) handleDeleteTweet(c *gin.Context) {
	claims := auth.MustClaimsFrom(c)
	if err := s.tweet.Delete(c.Request.Context(), c.Param("id"), claims.Sub); err != nil {
		httperr.Handle(c, err, s.log)
		return
	}
	c.Status(http.StatusNoContent)
}

func (s *Server) handleLike(c *gin.Context) {
	claims := auth.MustClaimsFrom(c)
	if _, err := s.tweet.Like(c.Request.Context(), claims.Sub, c.Param("id")); err != nil {
		httperr.Handle(c, err, s.log)
		return
	}
	c.JSON(http.StatusOK, likeResponse{Liked: true})
}

func (s *Server) handleUnlike(c *gin.Context) {
	claims := auth.MustClaimsFrom(c)
	if err := s.tweet.Unlike(c.Request.Context(), claims.Sub, c.Param("id")); err != nil {
		httperr.Handle(c, err, s.log)
		return
	}
	c.JSON(http.StatusOK, likeResponse{Liked: false})
}

func (s *Server) handleRetweet(c *gin.Context) {
	claims := auth.MustClaimsFrom(c)
	rt, err := s.tweet.Retweet(c.Request.Context(), claims.Sub, c.Param("id"))
	if err != nil {
		httperr.Handle(c, err, s.log)
		return
	}
	retweeter := s.tweet.FetchAuthor(c.Request.Context(), rt.RetweeterID)
	c.JSON(http.StatusCreated, retweetWrapper{
		ID:        rt.ID,
		Retweeter: authorInfoToResponse(retweeter),
		CreatedAt: rt.CreatedAt,
	})
}

func (s *Server) handleUnretweet(c *gin.Context) {
	claims := auth.MustClaimsFrom(c)
	if err := s.tweet.Unretweet(c.Request.Context(), claims.Sub, c.Param("id")); err != nil {
		httperr.Handle(c, err, s.log)
		return
	}
	c.JSON(http.StatusOK, retweetResponse{Retweeted: false})
}

func (s *Server) handleReply(c *gin.Context) {
	claims := auth.MustClaimsFrom(c)
	var req tweetBodyRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		httperr.Validation(c, err)
		return
	}
	t, err := s.tweet.Reply(c.Request.Context(), claims.Sub, c.Param("id"), req.Body, req.MediaID, req.MediaURL)
	if err != nil {
		httperr.Handle(c, err, s.log)
		return
	}
	author := s.tweet.FetchAuthor(c.Request.Context(), t.AuthorID)
	c.JSON(http.StatusCreated, newTweetResponse(tweetResponseParams{
		id:           t.ID,
		author:       author,
		body:         t.Body,
		replyToID:    t.ReplyToID,
		mediaID:      t.MediaID,
		mediaURL:     t.MediaUrl,
		likeCount:    t.LikeCount,
		retweetCount: t.RetweetCount,
		replyCount:   t.ReplyCount,
		createdAt:    t.CreatedAt,
	}))
}

func (s *Server) handleGetReplies(c *gin.Context) {
	claims := auth.MustClaimsFrom(c)
	limit := parseLimit(c)
	cursor := parseCursor(c)
	replies, err := s.tweet.GetReplies(c.Request.Context(), c.Param("id"), claims.Sub, limit, cursor)
	if err != nil {
		httperr.Handle(c, err, s.log)
		return
	}
	authorIDs := make([]string, len(replies))
	for i, r := range replies {
		authorIDs[i] = r.AuthorID
	}
	authors := s.tweet.FetchAuthors(c.Request.Context(), authorIDs)

	resp := make([]tweetResponse, len(replies))
	for i, r := range replies {
		resp[i] = tweetRowResponse(db.GetTweetByIDRow{
			ID:           r.ID,
			AuthorID:     r.AuthorID,
			Body:         r.Body,
			ReplyToID:    r.ReplyToID,
			MediaID:      r.MediaID,
			MediaUrl:     r.MediaUrl,
			LikeCount:    r.LikeCount,
			RetweetCount: r.RetweetCount,
			ReplyCount:   r.ReplyCount,
			CreatedAt:    r.CreatedAt,
			IsLiked:      r.IsLiked,
			IsRetweeted:  r.IsRetweeted,
		}, authors, nil)
	}
	var nextCursor *string
	if len(replies) == int(limit) {
		nextCursor = &replies[len(replies)-1].ID
	}
	c.JSON(http.StatusOK, tweetsResponse{Tweets: resp, NextCursor: nextCursor})
}

func (s *Server) handleGetProfileTimeline(c *gin.Context) {
	switch c.Query("filter") {
	case "replies":
		s.serveUserTimelineEntries(c, s.tweet.GetUserReplies)
	case "media":
		s.serveUserTimelineEntries(c, s.tweet.GetUserMedia)
	default:
		s.serveProfilePostsTimeline(c)
	}
}

func (s *Server) handleGetUserLikes(c *gin.Context) {
	claims := auth.MustClaimsFrom(c)
	if claims.Sub != c.Param("id") {
		httperr.Forbidden(c, "likes are visible only to the owner")
		return
	}
	s.serveUserTimelineEntries(c, s.tweet.GetUserLikes)
}

func (s *Server) serveProfilePostsTimeline(c *gin.Context) {
	limit := parseLimit(c)
	cursor := parseCursor(c)
	viewerID := ""
	if claims := auth.ClaimsFrom(c); claims != nil {
		viewerID = claims.Sub
	}
	userID := c.Param("id")

	rows, err := s.tweet.GetProfileTimeline(c.Request.Context(), userID, limit, cursor)
	if err != nil {
		httperr.Handle(c, err, s.log)
		return
	}

	tweetIDs := uniqueStrings(rows, func(r db.GetProfileTimelineByUserRow) string { return r.TweetID })
	tweetRows, err := s.batchFetchTweets(c.Request.Context(), tweetIDs, viewerID)
	if err != nil {
		httperr.Handle(c, err, s.log)
		return
	}

	authorIDSet := make(map[string]struct{}, len(rows))
	for _, t := range tweetRows {
		authorIDSet[t.AuthorID] = struct{}{}
	}
	for _, r := range rows {
		if r.RetweeterID != nil {
			authorIDSet[*r.RetweeterID] = struct{}{}
		}
	}
	authorIDs := make([]string, 0, len(authorIDSet))
	for id := range authorIDSet {
		authorIDs = append(authorIDs, id)
	}
	authors := s.tweet.FetchAuthors(c.Request.Context(), authorIDs)

	items := make([]feedItem, 0, len(rows))
	for _, r := range rows {
		t, ok := tweetRows[r.TweetID]
		if !ok {
			continue
		}
		body := tweetRowResponse(t, authors, nil)
		if r.Kind == "retweet" && r.RetweeterID != nil {
			items = append(items, feedItem{
				Kind:  "retweet",
				Tweet: body,
				Retweet: &retweetWrapper{
					ID:        r.ItemID,
					Retweeter: authorInfoToResponse(authors[*r.RetweeterID]),
					CreatedAt: r.SortAt,
				},
			})
			continue
		}
		items = append(items, feedItem{Kind: "tweet", Tweet: body})
	}

	var nextCursor *string
	if len(rows) == int(limit) {
		last := rows[len(rows)-1]
		cur := tweet.EncodeProfileCursor(last.SortAt, last.ItemID)
		nextCursor = &cur
	}
	c.JSON(http.StatusOK, feedResponse{Items: items, NextCursor: nextCursor})
}

func (s *Server) serveUserTimelineEntries(
	c *gin.Context,
	fetch func(ctx context.Context, userID string, limit int32, cursor *string) ([]tweet.TimelineEntry, error),
) {
	limit := parseLimit(c)
	cursor := parseCursor(c)
	viewerID := ""
	if claims := auth.ClaimsFrom(c); claims != nil {
		viewerID = claims.Sub
	}
	userID := c.Param("id")

	entries, err := fetch(c.Request.Context(), userID, limit, cursor)
	if err != nil {
		httperr.Handle(c, err, s.log)
		return
	}

	tweetIDs := uniqueStrings(entries, func(e tweet.TimelineEntry) string { return e.TweetID })
	tweetRows, err := s.batchFetchTweets(c.Request.Context(), tweetIDs, viewerID)
	if err != nil {
		httperr.Handle(c, err, s.log)
		return
	}

	authorIDSet := make(map[string]struct{}, len(tweetRows))
	for _, t := range tweetRows {
		authorIDSet[t.AuthorID] = struct{}{}
	}
	authorIDs := make([]string, 0, len(authorIDSet))
	for id := range authorIDSet {
		authorIDs = append(authorIDs, id)
	}
	authors := s.tweet.FetchAuthors(c.Request.Context(), authorIDs)

	items := make([]feedItem, 0, len(entries))
	for _, e := range entries {
		t, ok := tweetRows[e.TweetID]
		if !ok {
			continue
		}
		items = append(items, feedItem{Kind: "tweet", Tweet: tweetRowResponse(t, authors, nil)})
	}

	var nextCursor *string
	if len(entries) == int(limit) {
		last := entries[len(entries)-1]
		cur := tweet.EncodeProfileCursor(last.SortAt, last.TweetID)
		nextCursor = &cur
	}
	c.JSON(http.StatusOK, feedResponse{Items: items, NextCursor: nextCursor})
}

func uniqueStrings[T any](rows []T, key func(T) string) []string {
	seen := make(map[string]struct{}, len(rows))
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		k := key(r)
		if k == "" {
			continue
		}
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, k)
	}
	return out
}

func (s *Server) batchFetchTweets(ctx context.Context, ids []string, viewerID string) (map[string]db.GetTweetByIDRow, error) {
	out := make(map[string]db.GetTweetByIDRow, len(ids))
	for _, id := range ids {
		t, err := s.tweet.GetByID(ctx, id, viewerID)
		if err != nil {
			continue
		}
		out[id] = t
	}
	return out, nil
}
