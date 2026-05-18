-- name: CreateTweet :one
INSERT INTO tweets (id, author_id, body, media_id, media_url)
VALUES ($1, $2, $3, $4, $5)
RETURNING *;

-- name: CreateReply :one
INSERT INTO tweets (id, author_id, body, reply_to_id, media_id, media_url)
VALUES ($1, $2, $3, $4, $5, $6)
RETURNING *;

-- name: GetTweetByID :one
SELECT t.*,
  CASE WHEN sqlc.narg(viewer_id)::TEXT IS NOT NULL THEN
    EXISTS(SELECT 1 FROM likes WHERE user_id = sqlc.narg(viewer_id)::TEXT AND tweet_id = t.id)
  ELSE false END AS is_liked,
  CASE WHEN sqlc.narg(viewer_id)::TEXT IS NOT NULL THEN
    EXISTS(SELECT 1 FROM retweets rt WHERE rt.original_tweet_id = t.id AND rt.retweeter_id = sqlc.narg(viewer_id)::TEXT)
  ELSE false END AS is_retweeted
FROM tweets t WHERE t.id = $1;

-- name: DeleteTweet :exec
DELETE FROM tweets WHERE id = $1 AND author_id = $2;

-- name: GetTweetsByAuthor :many
SELECT t.*,
  CASE WHEN sqlc.narg(viewer_id)::TEXT IS NOT NULL THEN
    EXISTS(SELECT 1 FROM likes WHERE user_id = sqlc.narg(viewer_id)::TEXT AND tweet_id = t.id)
  ELSE false END AS is_liked,
  CASE WHEN sqlc.narg(viewer_id)::TEXT IS NOT NULL THEN
    EXISTS(SELECT 1 FROM retweets rt WHERE rt.original_tweet_id = t.id AND rt.retweeter_id = sqlc.narg(viewer_id)::TEXT)
  ELSE false END AS is_retweeted
FROM tweets t
WHERE t.author_id = sqlc.arg(author_id)
  AND (sqlc.narg(cursor)::text IS NULL OR t.id < sqlc.narg(cursor)::text)
ORDER BY t.id DESC
LIMIT sqlc.arg(page_limit)::int4;

-- name: GetRepliesByTweetID :many
SELECT t.*,
  CASE WHEN sqlc.narg(viewer_id)::TEXT IS NOT NULL THEN
    EXISTS(SELECT 1 FROM likes WHERE user_id = sqlc.narg(viewer_id)::TEXT AND tweet_id = t.id)
  ELSE false END AS is_liked,
  CASE WHEN sqlc.narg(viewer_id)::TEXT IS NOT NULL THEN
    EXISTS(SELECT 1 FROM retweets rt WHERE rt.original_tweet_id = t.id AND rt.retweeter_id = sqlc.narg(viewer_id)::TEXT)
  ELSE false END AS is_retweeted
FROM tweets t
WHERE t.reply_to_id = sqlc.arg(reply_to_id)
  AND (sqlc.narg(cursor)::text IS NULL OR t.id > sqlc.narg(cursor)::text)
ORDER BY t.id ASC
LIMIT sqlc.arg(page_limit)::int4;

-- name: IncrementLikeCount :exec
UPDATE tweets SET like_count = like_count + 1 WHERE id = $1;

-- name: DecrementLikeCount :exec
UPDATE tweets SET like_count = GREATEST(like_count - 1, 0) WHERE id = $1;

-- name: IncrementRetweetCount :exec
UPDATE tweets SET retweet_count = retweet_count + 1 WHERE id = $1;

-- name: DecrementRetweetCount :exec
UPDATE tweets SET retweet_count = GREATEST(retweet_count - 1, 0) WHERE id = $1;

-- name: IncrementReplyCount :exec
UPDATE tweets SET reply_count = reply_count + 1 WHERE id = $1;

-- name: DecrementReplyCount :exec
UPDATE tweets SET reply_count = GREATEST(reply_count - 1, 0) WHERE id = $1;

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

-- name: GetRecentPopularTweets :many
SELECT id, author_id, body, like_count, retweet_count, reply_count, created_at
FROM tweets
WHERE created_at >= NOW() - INTERVAL '48 hours'
  AND reply_to_id IS NULL
ORDER BY (like_count + retweet_count * 2 + reply_count) DESC, created_at DESC
LIMIT sqlc.arg(page_limit)::int4;

-- name: GetInteractionsByIDs :many
-- Returns viewer-specific is_liked and is_retweeted flags for a batch of tweet IDs.
-- Counts are NOT included; they come from Redis tweet:counts:{id}.
SELECT
  tw.id AS tweet_id,
  EXISTS(SELECT 1 FROM likes l WHERE l.user_id = sqlc.arg(viewer_id) AND l.tweet_id = tw.id) AS is_liked,
  EXISTS(SELECT 1 FROM retweets rt WHERE rt.original_tweet_id = tw.id AND rt.retweeter_id = sqlc.arg(viewer_id)) AS is_retweeted
FROM tweets tw
WHERE tw.id = ANY(sqlc.arg(tweet_ids)::text[]);

-- name: CreateLike :execresult
INSERT INTO likes (user_id, tweet_id) VALUES ($1, $2)
ON CONFLICT DO NOTHING;

-- name: DeleteLike :exec
DELETE FROM likes WHERE user_id = $1 AND tweet_id = $2;

-- name: InsertRetweet :one
INSERT INTO retweets (id, retweeter_id, original_tweet_id)
VALUES ($1, $2, $3)
RETURNING *;

-- name: DeleteRetweetByPair :exec
DELETE FROM retweets WHERE retweeter_id = $1 AND original_tweet_id = $2;

-- name: GetRetweetByPair :one
SELECT * FROM retweets WHERE retweeter_id = $1 AND original_tweet_id = $2;

-- name: IsRetweetedBy :one
SELECT EXISTS(
  SELECT 1 FROM retweets WHERE retweeter_id = $1 AND original_tweet_id = $2
) AS is_retweeted;

-- name: GetProfileTimelineByUser :many
-- Mixed feed of a user's own tweets and their retweets, ordered by recency.
-- Cursor is composite "<unix_micros>|<item_id>" — see service layer.
SELECT
    'tweet'::text       AS kind,
    t.id                AS item_id,
    t.id                AS tweet_id,
    NULL::text          AS retweeter_id,
    t.created_at        AS sort_at
FROM tweets t
WHERE t.author_id = sqlc.arg(user_id)
  AND t.reply_to_id IS NULL
  AND (
    sqlc.narg(cursor_at)::timestamptz IS NULL
    OR t.created_at < sqlc.narg(cursor_at)::timestamptz
    OR (t.created_at = sqlc.narg(cursor_at)::timestamptz AND t.id < sqlc.narg(cursor_id)::text)
  )
UNION ALL
SELECT
    'retweet'::text     AS kind,
    r.id                AS item_id,
    r.original_tweet_id AS tweet_id,
    r.retweeter_id      AS retweeter_id,
    r.created_at        AS sort_at
FROM retweets r
WHERE r.retweeter_id = sqlc.arg(user_id)
  AND (
    sqlc.narg(cursor_at)::timestamptz IS NULL
    OR r.created_at < sqlc.narg(cursor_at)::timestamptz
    OR (r.created_at = sqlc.narg(cursor_at)::timestamptz AND r.id < sqlc.narg(cursor_id)::text)
  )
ORDER BY sort_at DESC, item_id DESC
LIMIT sqlc.arg(page_limit)::int4;

-- name: GetUserReplies :many
-- Replies authored by a user, oldest replies last (most recent first).
-- Standalone reply rows — parent context is NOT joined (frontend renders flat).
SELECT
    t.id         AS item_id,
    t.id         AS tweet_id,
    t.created_at AS sort_at
FROM tweets t
WHERE t.author_id = sqlc.arg(user_id)
  AND t.reply_to_id IS NOT NULL
  AND (
    sqlc.narg(cursor_at)::timestamptz IS NULL
    OR t.created_at < sqlc.narg(cursor_at)::timestamptz
    OR (t.created_at = sqlc.narg(cursor_at)::timestamptz AND t.id < sqlc.narg(cursor_id)::text)
  )
ORDER BY t.created_at DESC, t.id DESC
LIMIT sqlc.arg(page_limit)::int4;

-- name: GetUserMedia :many
-- Tweets with media authored by a user, across both tweets and replies.
-- Backed by idx_tweets_author_media partial index.
SELECT
    t.id         AS item_id,
    t.id         AS tweet_id,
    t.created_at AS sort_at
FROM tweets t
WHERE t.author_id = sqlc.arg(user_id)
  AND t.media_id IS NOT NULL
  AND (
    sqlc.narg(cursor_at)::timestamptz IS NULL
    OR t.created_at < sqlc.narg(cursor_at)::timestamptz
    OR (t.created_at = sqlc.narg(cursor_at)::timestamptz AND t.id < sqlc.narg(cursor_id)::text)
  )
ORDER BY t.created_at DESC, t.id DESC
LIMIT sqlc.arg(page_limit)::int4;

-- name: GetUserLikes :many
-- Tweets a user has liked, ordered by like time (NOT tweet creation time).
-- Cursor sorts on likes.created_at so pagination is stable across new tweets.
SELECT
    l.tweet_id   AS item_id,
    l.tweet_id   AS tweet_id,
    l.created_at AS sort_at
FROM likes l
WHERE l.user_id = sqlc.arg(user_id)
  AND (
    sqlc.narg(cursor_at)::timestamptz IS NULL
    OR l.created_at < sqlc.narg(cursor_at)::timestamptz
    OR (l.created_at = sqlc.narg(cursor_at)::timestamptz AND l.tweet_id < sqlc.narg(cursor_id)::text)
  )
ORDER BY l.created_at DESC, l.tweet_id DESC
LIMIT sqlc.arg(page_limit)::int4;
