package notification

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"

	"github.com/redis/go-redis/v9"
	db "github.com/twitter/notification-service/db/sqlc"
)

const redisPubSubChannel = "notifications"

type redisMessage struct {
	UserID string          `json:"user_id"`
	Notif  db.Notification `json:"notif"`
}

// Hub is an SSE fan-out backed by Redis Pub/Sub for multi-pod support.
type Hub struct {
	mu   sync.RWMutex
	subs map[string][]chan db.Notification
	rdb  *redis.Client
	log  *slog.Logger
}

func NewHub(rdb *redis.Client, log *slog.Logger) *Hub {
	return &Hub{
		subs: make(map[string][]chan db.Notification),
		rdb:  rdb,
		log:  log,
	}
}

func (h *Hub) Subscribe(userID string) chan db.Notification {
	ch := make(chan db.Notification, 8)
	h.mu.Lock()
	defer h.mu.Unlock()
	h.subs[userID] = append(h.subs[userID], ch)
	return ch
}

func (h *Hub) Unsubscribe(userID string, ch chan db.Notification) {
	h.mu.Lock()
	defer h.mu.Unlock()
	subs := h.subs[userID]
	for i, s := range subs {
		if s == ch {
			h.subs[userID] = append(subs[:i], subs[i+1:]...)
			break
		}
	}
	close(ch)
}

// If rdb is nil (tests), the notification is fanned out in-process only.
func (h *Hub) Publish(ctx context.Context, userID string, n db.Notification) {
	if h.rdb == nil {
		h.fanOutLocal(userID, n)
		return
	}
	b, err := json.Marshal(redisMessage{UserID: userID, Notif: n})
	if err != nil {
		h.log.Error("hub publish marshal", "err", err)
		return
	}
	if err := h.rdb.Publish(ctx, redisPubSubChannel, b).Err(); err != nil {
		h.log.Error("hub redis publish", "err", err)
	}
}

// No-ops if rdb is nil.
func (h *Hub) RunPubSub(ctx context.Context) {
	if h.rdb == nil {
		<-ctx.Done()
		return
	}
	sub := h.rdb.Subscribe(ctx, redisPubSubChannel)
	defer sub.Close()

	ch := sub.Channel()
	for {
		select {
		case <-ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				return
			}
			var rm redisMessage
			if err := json.Unmarshal([]byte(msg.Payload), &rm); err != nil {
				h.log.Error("hub pubsub unmarshal", "err", err)
				continue
			}
			h.fanOutLocal(rm.UserID, rm.Notif)
		}
	}
}

func (h *Hub) fanOutLocal(userID string, n db.Notification) {
	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, ch := range h.subs[userID] {
		select {
		case ch <- n:
		default:
		}
	}
}
