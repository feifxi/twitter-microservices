package server

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/twitter/shared/auth"
	"github.com/twitter/shared/httperr"
	db "github.com/twitter/notification-service/db/sqlc"
)

const (
	defaultNotifLimit = int32(20)
	maxNotifLimit     = int32(100)
	minNotifLimit     = int32(1)
)

func (s *Server) handleGetNotifications(c *gin.Context) {
	claims := auth.MustClaimsFrom(c)
	var cursor *string
	if raw := c.Query("cursor"); raw != "" {
		cursor = &raw
	}
	limit := defaultNotifLimit
	if l := c.Query("limit"); l != "" {
		n, err := strconv.Atoi(l)
		if err != nil || int32(n) < minNotifLimit || int32(n) > maxNotifLimit {
			httperr.BadRequest(c, "INVALID_PARAM", "limit must be between 1 and 100")
			return
		}
		limit = int32(n)
	}

	notifs, err := s.notif.GetNotifications(c.Request.Context(), claims.Sub, cursor, limit)
	if err != nil {
		httperr.Handle(c, err, s.log)
		return
	}

	var nextCursor *string
	if len(notifs) == int(limit) {
		id := notifs[len(notifs)-1].ID
		nextCursor = &id
	}

	actorIDs := make([]string, len(notifs))
	tweetIDs := make([]string, 0, len(notifs))
	for i, n := range notifs {
		actorIDs[i] = n.ActorID
		if n.TweetID != nil {
			tweetIDs = append(tweetIDs, *n.TweetID)
		}
	}
	actors := s.lookupActors(c.Request.Context(), actorIDs)
	previews := s.lookupTweetPreviews(c.Request.Context(), tweetIDs)

	resp := make([]notificationResponse, len(notifs))
	for i, n := range notifs {
		resp[i] = newNotificationResponse(n, actors, previews)
	}
	c.JSON(http.StatusOK, notificationsResponse{Notifications: resp, NextCursor: nextCursor})
}

func (s *Server) handleMarkAllRead(c *gin.Context) {
	claims := auth.MustClaimsFrom(c)
	if err := s.notif.MarkAllRead(c.Request.Context(), claims.Sub); err != nil {
		httperr.Handle(c, err, s.log)
		return
	}
	c.JSON(http.StatusOK, markReadResponse{Read: true})
}

func (s *Server) handleMarkRead(c *gin.Context) {
	claims := auth.MustClaimsFrom(c)
	n, err := s.notif.MarkRead(c.Request.Context(), c.Param("id"), claims.Sub)
	if err != nil {
		httperr.Handle(c, err, s.log)
		return
	}
	actors := s.lookupActors(c.Request.Context(), []string{n.ActorID})
	var tweetIDs []string
	if n.TweetID != nil {
		tweetIDs = []string{*n.TweetID}
	}
	previews := s.lookupTweetPreviews(c.Request.Context(), tweetIDs)
	c.JSON(http.StatusOK, newNotificationResponse(n, actors, previews))
}

func (s *Server) handleStream(c *gin.Context) {
	claims := auth.MustClaimsFrom(c)
	ch := s.hub.Subscribe(claims.Sub)
	defer s.hub.Unsubscribe(claims.Sub, ch)

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	w := c.Writer
	flusher, ok := w.(http.Flusher)
	if !ok {
		httperr.Internal(c)
		return
	}

	// Send initial unread count so client can sync badge without a separate request.
	if count, err := s.notif.CountUnread(c.Request.Context(), claims.Sub); err == nil {
		if b, err := json.Marshal(unreadCountEvent{Count: count}); err == nil {
			fmt.Fprintf(w, "event: count\ndata: %s\n\n", b)
		}
	}
	fmt.Fprintf(w, ": heartbeat\n\n")
	flusher.Flush()

	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	for {
		select {
		case n, ok := <-ch:
			if !ok {
				return
			}
			writeSSE(w, flusher, n, s.lookupActor(c.Request.Context(), n.ActorID), s.lookupTweetPreview(c.Request.Context(), func() string {
				if n.TweetID != nil {
					return *n.TweetID
				}
				return ""
			}()))

		case <-heartbeat.C:
			fmt.Fprintf(w, ": heartbeat\n\n")
			flusher.Flush()

		case <-c.Request.Context().Done():
			return
		}
	}
}

func writeSSE(w gin.ResponseWriter, flusher http.Flusher, n db.Notification, actor actorInfo, preview *string) {
	payload, err := json.Marshal(newSSENotification(n, actor, preview))
	if err != nil {
		return
	}
	fmt.Fprintf(w, "id: %s\nevent: notification\ndata: %s\n\n", n.ID, payload)
	flusher.Flush()
}
