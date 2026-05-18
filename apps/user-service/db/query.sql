-- ── Outbox ────────────────────────────────────────────────────────────────────

-- name: InsertOutbox :exec
INSERT INTO outbox (topic, key, payload, trace_id) VALUES ($1, $2, $3, $4);

-- name: GetPendingOutbox :many
SELECT id, topic, key, payload, trace_id FROM outbox
WHERE sent_at IS NULL
ORDER BY id ASC
LIMIT 100
FOR UPDATE SKIP LOCKED;

-- name: MarkOutboxSent :exec
UPDATE outbox SET sent_at = now() WHERE id = $1;

-- ── Users ────────────────────────────────────────────────────────────────────

-- name: ProvisionUser :one
INSERT INTO users (id, email, display_name)
VALUES ($1, $2, $3)
RETURNING *;

-- name: GetUserByID :one
SELECT * FROM users WHERE id = $1;

-- name: UpdateProfile :one
UPDATE users
SET username         = COALESCE(sqlc.narg(username)::TEXT,         username),
    display_name     = COALESCE(sqlc.narg(display_name)::TEXT,     display_name),
    avatar_url       = COALESCE(sqlc.narg(avatar_url)::TEXT,       avatar_url),
    header_image_url = COALESCE(sqlc.narg(header_image_url)::TEXT, header_image_url),
    bio              = COALESCE(sqlc.narg(bio)::TEXT,              bio),
    website_url      = COALESCE(sqlc.narg(website_url)::TEXT,      website_url),
    location         = COALESCE(sqlc.narg(location)::TEXT,         location),
    updated_at       = now()
WHERE id = sqlc.arg(id)
RETURNING *;

-- name: IncrementFollowerCount :exec
UPDATE users SET follower_count = follower_count + 1 WHERE id = $1;

-- name: DecrementFollowerCount :exec
UPDATE users SET follower_count = GREATEST(follower_count - 1, 0) WHERE id = $1;

-- name: IncrementFollowingCount :exec
UPDATE users SET following_count = following_count + 1 WHERE id = $1;

-- name: DecrementFollowingCount :exec
UPDATE users SET following_count = GREATEST(following_count - 1, 0) WHERE id = $1;

-- ── Follows ───────────────────────────────────────────────────────────────────

-- name: CreateFollow :execresult
INSERT INTO follows (follower_id, followee_id)
VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: DeleteFollow :exec
DELETE FROM follows WHERE follower_id = $1 AND followee_id = $2;

-- name: IsFollowing :one
SELECT EXISTS (
    SELECT 1 FROM follows
    WHERE follower_id = $1 AND followee_id = $2
) AS is_following;

-- name: GetFollowerCount :one
SELECT COUNT(*) AS count FROM follows WHERE followee_id = $1;

-- name: BatchGetFollowerCounts :many
-- Reads from the denormalized follower_count column on users (HINCRBY-style
-- counter maintained by Increment/DecrementFollowerCount). Cheap point lookup
-- by primary key for each id.
SELECT id, follower_count FROM users WHERE id = ANY(sqlc.arg(user_ids)::TEXT[]);

-- name: GetFollowState :many
-- Returns the subset of target_ids that viewer_id currently follows. Caller
-- defaults missing entries to false.
SELECT followee_id FROM follows
WHERE follower_id = sqlc.arg(viewer_id)
  AND followee_id = ANY(sqlc.arg(target_ids)::TEXT[]);

-- name: GetFollowerIDs :many
SELECT follower_id FROM follows
WHERE followee_id = $1
ORDER BY created_at DESC;

-- name: GetFollowingIDs :many
SELECT followee_id FROM follows
WHERE follower_id = $1
ORDER BY created_at DESC;

-- name: ListFollowers :many
-- Lists accounts that follow user_id, ordered by most-recently-followed first.
-- viewer_id is the requester — used to compute is_following_viewer so the UI
-- can render Follow/Following buttons without a second round-trip.
-- Cursor is the composite (f.created_at, f.follower_id) of the last row of
-- the previous page; ties on created_at are broken by follower_id.
SELECT
    u.*,
    f.created_at AS followed_at,
    CASE WHEN sqlc.narg(viewer_id)::TEXT IS NOT NULL THEN
        EXISTS (
            SELECT 1 FROM follows v
            WHERE v.follower_id = sqlc.narg(viewer_id)::TEXT
              AND v.followee_id = u.id
        )
    ELSE false END AS is_following_viewer
FROM follows f
JOIN users u ON u.id = f.follower_id
WHERE f.followee_id = sqlc.arg(user_id)
  AND (
    sqlc.narg(cursor_at)::timestamptz IS NULL
    OR f.created_at < sqlc.narg(cursor_at)::timestamptz
    OR (f.created_at = sqlc.narg(cursor_at)::timestamptz
        AND f.follower_id < sqlc.narg(cursor_id)::text)
  )
ORDER BY f.created_at DESC, f.follower_id DESC
LIMIT sqlc.arg(page_limit)::int4;

-- name: ListFollowing :many
-- Lists accounts that user_id follows. Same cursor + is_following_viewer pattern
-- as ListFollowers, but joins on followee_id.
SELECT
    u.*,
    f.created_at AS followed_at,
    CASE WHEN sqlc.narg(viewer_id)::TEXT IS NOT NULL THEN
        EXISTS (
            SELECT 1 FROM follows v
            WHERE v.follower_id = sqlc.narg(viewer_id)::TEXT
              AND v.followee_id = u.id
        )
    ELSE false END AS is_following_viewer
FROM follows f
JOIN users u ON u.id = f.followee_id
WHERE f.follower_id = sqlc.arg(user_id)
  AND (
    sqlc.narg(cursor_at)::timestamptz IS NULL
    OR f.created_at < sqlc.narg(cursor_at)::timestamptz
    OR (f.created_at = sqlc.narg(cursor_at)::timestamptz
        AND f.followee_id < sqlc.narg(cursor_id)::text)
  )
ORDER BY f.created_at DESC, f.followee_id DESC
LIMIT sqlc.arg(page_limit)::int4;

-- name: ListUserSuggestions :many
-- "Who to follow" — friend-of-friend ranking. For each candidate user, score
-- is the number of viewer's followees who also follow them. Excludes the
-- viewer, accounts the viewer already follows, and un-onboarded users
-- (username IS NULL). Falls back gracefully for new accounts: candidates that
-- nobody-the-viewer-follows follows get score=0 and are sorted by global
-- follower_count, so brand-new users still see the most-followed accounts.
SELECT
    u.*,
    COUNT(f.follower_id)::int AS mutual_count
FROM users u
LEFT JOIN follows f
    ON f.followee_id = u.id
   AND f.follower_id IN (
        SELECT vf.followee_id FROM follows vf
        WHERE vf.follower_id = sqlc.arg(viewer_id)
   )
WHERE u.id != sqlc.arg(viewer_id)
  AND u.username IS NOT NULL
  AND NOT EXISTS (
    SELECT 1 FROM follows xf
    WHERE xf.follower_id = sqlc.arg(viewer_id) AND xf.followee_id = u.id
  )
GROUP BY u.id
ORDER BY mutual_count DESC, u.follower_count DESC, u.id ASC
LIMIT sqlc.arg(page_limit)::int4;
