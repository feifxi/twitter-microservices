-- name: CreateNotification :one
INSERT INTO notifications (id, user_id, actor_id, type, tweet_id)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, user_id, actor_id, type, tweet_id, read_at, created_at;

-- name: GetNotificationsByUserID :many
SELECT n.id, n.user_id, n.actor_id, n.type, n.tweet_id, n.read_at, n.created_at
FROM notifications n
WHERE n.user_id = sqlc.arg(user_id)
  AND (sqlc.narg(cursor)::text IS NULL OR n.id < sqlc.narg(cursor)::text)
ORDER BY n.id DESC
LIMIT sqlc.arg(page_limit)::int4;

-- name: MarkNotificationRead :one
UPDATE notifications
SET read_at = COALESCE(read_at, NOW())
WHERE id = $1 AND user_id = $2
RETURNING id, user_id, actor_id, type, tweet_id, read_at, created_at;

-- name: MarkAllNotificationsRead :exec
UPDATE notifications
SET read_at = NOW()
WHERE user_id = $1 AND read_at IS NULL;

-- name: DeleteRetweetNotification :exec
-- Removes the "X retweeted your tweet" notification when X unretweets.
-- Matches on (actor, original tweet, type='retweet') — at most one such row
-- exists because (actor, tweet, type) is naturally unique for retweets.
DELETE FROM notifications
WHERE actor_id = sqlc.arg(actor_id)
  AND tweet_id = sqlc.arg(tweet_id)
  AND type     = 'retweet';

-- name: CountUnreadNotifications :one
SELECT COUNT(*)::int AS count FROM notifications WHERE user_id = $1 AND read_at IS NULL;

-- name: IsEventProcessed :one
SELECT EXISTS(SELECT 1 FROM processed_events WHERE event_id = $1)::bool;

-- name: MarkEventProcessed :exec
INSERT INTO processed_events (event_id) VALUES ($1) ON CONFLICT DO NOTHING;

-- name: PruneProcessedEvents :execrows
-- Removes dedup rows older than $1 (typically 7 days). Run on a daily ticker;
-- the broker will never redeliver a message that old, so older rows are safe
-- to drop.
DELETE FROM processed_events WHERE processed_at < NOW() - sqlc.arg(older_than)::interval;
