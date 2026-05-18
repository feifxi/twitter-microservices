package server

import (
	"time"

	db "github.com/twitter/notification-service/db/sqlc"
)

type healthzResponse struct {
	Status  string `json:"status"`
	Service string `json:"service"`
}

type actorInfo struct {
	ID          string
	Username    string
	DisplayName string
	AvatarURL   string
}

type notificationResponse struct {
	ID               string     `json:"id"`
	ActorID          string     `json:"actor_id"`
	ActorUsername    string     `json:"actor_username"`
	ActorDisplayName string     `json:"actor_display_name"`
	ActorAvatarURL   *string    `json:"actor_avatar_url"`
	Type             string     `json:"type"`
	TweetID          *string    `json:"tweet_id"`
	TweetPreview     *string    `json:"tweet_preview"`
	ReadAt           *time.Time `json:"read_at"`
	CreatedAt        time.Time  `json:"created_at"`
}

type notificationsResponse struct {
	Notifications []notificationResponse `json:"notifications"`
	NextCursor    *string                `json:"next_cursor"`
}

type markReadResponse struct {
	Read bool `json:"read"`
}

type unreadCountEvent struct {
	Count int32 `json:"count"`
}

type sseNotification struct {
	ID               string    `json:"id"`
	ActorID          string    `json:"actor_id"`
	ActorUsername    string    `json:"actor_username"`
	ActorDisplayName string    `json:"actor_display_name"`
	ActorAvatarURL   *string   `json:"actor_avatar_url"`
	Type             string    `json:"type"`
	TweetID          *string   `json:"tweet_id"`
	TweetPreview     *string   `json:"tweet_preview"`
	CreatedAt        time.Time `json:"created_at"`
}

func newNotificationResponse(n db.Notification, actors map[string]actorInfo, previews map[string]*string) notificationResponse {
	r := notificationResponse{
		ID:        n.ID,
		ActorID:   n.ActorID,
		Type:      n.Type,
		TweetID:   n.TweetID,
		CreatedAt: n.CreatedAt,
	}
	if n.ReadAt.Valid {
		r.ReadAt = &n.ReadAt.Time
	}
	if actor, ok := actors[n.ActorID]; ok {
		r.ActorUsername = actor.Username
		r.ActorDisplayName = actor.DisplayName
		if actor.AvatarURL != "" {
			r.ActorAvatarURL = &actor.AvatarURL
		}
	}
	if n.TweetID != nil {
		r.TweetPreview = previews[*n.TweetID]
	}
	return r
}

func newSSENotification(n db.Notification, actor actorInfo, preview *string) sseNotification {
	s := sseNotification{
		ID:               n.ID,
		ActorID:          n.ActorID,
		ActorUsername:    actor.Username,
		ActorDisplayName: actor.DisplayName,
		Type:             n.Type,
		TweetID:          n.TweetID,
		TweetPreview:     preview,
		CreatedAt:        n.CreatedAt,
	}
	if actor.AvatarURL != "" {
		s.ActorAvatarURL = &actor.AvatarURL
	}
	return s
}
