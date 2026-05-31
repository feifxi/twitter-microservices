# Twitter Clone — Architecture

## Table of Contents

1. [Overview](#overview)
2. [System Architecture](#system-architecture)
3. [Monorepo Layout](#monorepo-layout)
4. [Component Decisions & Tradeoffs](#component-decisions--tradeoffs)
5. [Services & API Contracts](#services--api-contracts)
6. [Error Handling](#error-handling)
7. [Service Dependency Map](#service-dependency-map)
8. [Data Models](#data-models)
9. [Kafka Events](#kafka-events)
10. [Redis Key Design](#redis-key-design)
11. [Authentication & Gateway](#authentication--gateway)
12. [Core Patterns](#core-patterns)
    - Transactional Outbox
    - Fan-out & Timeline
    - Celebrity Merge
    - Cursor Pagination
    - Trending Hashtags
    - Recommended Feed
    - Real-time Notifications (SSE)
13. [Frontend Architecture](#frontend-architecture)
14. [Infrastructure](#infrastructure)
15. [Observability & Resilience](#observability--resilience)
16. [ID Conventions](#id-conventions)

---

## Overview

A Twitter-like platform built as Go microservices on AWS EKS. Users post tweets with hashtags, retweet, follow each other, get a personalised home feed, receive real-time notifications, upload media, and search tweets by keyword or semantic meaning.

**Primary goal:** correctness and observability first; horizontal scale is a secondary concern at current traffic.

**Stack:** Go 1.26 + Gin · Next.js 16.2 (App Router) · PostgreSQL (Aurora Serverless v2) · Redis (ElastiCache) · Kafka (Amazon MSK) · OpenSearch 3.5 · S3 · Kong 3.6 · gRPC · AWS EKS · Terraform

---

## System Architecture

```
                         ┌─────────────────────────────────────────────┐
                         │                  INTERNET                    │
                         └───────────────────┬─────────────────────────┘
                                             │
                         ┌───────────────────▼─────────────────────────┐
                         │              Route 53 (DNS)                  │
                         └────────┬────────────────────┬────────────────┘
                                  │                    │
              ┌───────────────────▼────────┐  ┌────────▼────────────────────┐
              │  CloudFront (Next.js SSR)  │  │  CloudFront + S3 (media CDN)│
              └───────────────────┬────────┘  └─────────────────────────────┘
                                  │
              ┌───────────────────▼────────┐
              │    ALB + WAF (HTTPS :443)  │
              └───────────────────┬────────┘
                                  │
     ┌────────────────────────────▼──────────────────────────────────────┐
     │                      Amazon EKS Cluster                            │
     │                                                                    │
     │  ┌─────────────────────────────────────────────────────────────┐  │
     │  │  Kong 3.6 (DB-less, declarative)                            │  │
     │  │  JWT validation → X-User-* header injection                 │  │
     │  └─────────────────────────┬───────────────────────────────────┘  │
     │                            │  internal network only               │
     │  ┌─────────────────────────▼───────────────────────────────────┐  │
     │  │  user-service  tweet-service  feed-service  notif-service   │  │
     │  │  media-service  search-service                               │  │
     │  └──────────────────────────────────────────────────────────────┘  │
     └────────────────────────────────────────────────────────────────────┘
                                  │
        ┌─────────────────────────┼──────────────────────────┐
        │                         │                          │
┌───────▼────────────┐  ┌─────────▼───────────┐  ┌──────────▼──────────┐
│  Aurora PG         │  │  ElastiCache Redis  │  │  Amazon MSK (Kafka) │
│  Serverless v2     │  │  (shared instance)  │  │  (shared cluster)   │
│  schema-per-svc    │  │                     │  │                     │
└────────────────────┘  └─────────────────────┘  └─────────────────────┘

┌────────────────────────────────────────────────────────────────────────┐
│  Amazon OpenSearch Service 3.5                                          │
│  index: tweets (BM25 + kNN 256-dim hybrid)  ·  index: users (BM25)     │
└────────────────────────────────────────────────────────────────────────┘
```

**Key structural rules:**
- **Kong is the only internet-facing entry point.** Services are unreachable directly via K8s network policy. This means JWT validation, rate limiting, and header injection happen once at the gateway, not inside each service.
- **No cross-service database access.** Each service owns its schema exclusively. Data sharing happens through Kafka events or Redis (denormalised snapshots), never through shared DB tables.
- **gRPC for synchronous service-to-service calls.** REST is only for Kong-facing public APIs.

---

## Monorepo Layout

```
/
├── apps/
│   ├── web/                  # Next.js 16.2 frontend
│   ├── shared/               # Shared Go module imported by all services
│   │   ├── auth/             # Kong-header middleware (HeadersMiddleware, Claims)
│   │   ├── httperr/          # Unified error type + all handler helpers
│   │   ├── events/           # Kafka topic constants + event struct definitions
│   │   ├── outbox/           # Generic outbox flusher (polling, batching, headers)
│   │   ├── dlq/              # Dead-letter Kafka writer (provenance headers)
│   │   ├── dbmigrate/        # golang-migrate runner (strips search_path before migrating)
│   │   ├── pgxutil/          # MapErr: pgx → httperr sentinel mapping
│   │   ├── ptr/              # Pointer helpers (NonEmpty, Deref)
│   │   ├── envutil/          # MustEnv, GetEnv
│   │   ├── logger/           # slog JSON setup
│   │   ├── metrics/          # Prometheus counters/histograms + Gin / gRPC middleware
│   │   ├── otel/             # OTLP exporter setup
│   │   ├── reqlog/           # request_id / trace context / structured access log
│   │   ├── tracing/          # Kafka header propagation
│   │   └── proto/            # .proto files + buf config + generated Go code
│   ├── user-service/
│   ├── tweet-service/
│   ├── feed-service/
│   ├── notification-service/
│   ├── media-service/
│   └── search-service/
├── local/
│   ├── kong/kong.yaml          # Declarative Kong config (dev RSA key, Lua scripts)
│   ├── keycloak/               # realm-twitter.json (rendered from template via make generate-realm)
│   ├── keycloak-spi/           # Provision User Required Action SPI (Java, built into a JAR)
│   ├── postgres/init/          # Per-schema init SQL (creates databases for Keycloak + services)
│   ├── opensearch/             # opensearch.yml (single-node settings for local)
│   ├── prometheus/             # prometheus.yml scrape config
│   └── localstack/             # S3 bucket init script
├── infra/
│   ├── terraform/            # AWS infrastructure
│   └── k8s/                  # Helm charts per service
├── docs/
│   └── ARCHITECTURE.md       # this file
├── go.work                   # Go workspace — ties all service modules together
└── docker-compose.yml
```

**Service internal layout** (identical for all Go services):
```
apps/{service}/
├── cmd/server/main.go        # wiring only — connect deps, start server, graceful shutdown
├── internal/
│   ├── server/
│   │   ├── server.go         # Server struct, route registration
│   │   ├── handler_*.go      # Thin HTTP handlers: bind → call service → respond
│   │   ├── types.go          # ALL request/response structs (never inline anonymous structs)
│   │   └── grpc.go           # gRPC handlers (user-service, tweet-service only)
│   └── {domain}/service.go   # All business logic — no HTTP/gRPC types here
├── db/
│   ├── query.sql             # Named sqlc queries (edit this, not generated code)
│   └── sqlc/                 # Generated code + hand-written store.go (Store/ExecTx)
└── migrations/               # golang-migrate SQL files
```

---

## Component Decisions & Tradeoffs

| Component | Choice | Why | Tradeoff |
|-----------|--------|-----|----------|
| API Gateway | **Kong 3.6** (DB-less) | Single place for JWT validation, rate limiting, correlation IDs, header injection | An extra network hop; config is declarative YAML, harder to change dynamically than DB mode |
| Auth | **Keycloak 26** | Full OAuth 2.0/OIDC with Google federation; user-service stays focused on profiles | Self-hosted IAM adds operational complexity vs managed auth (Auth0/Cognito) |
| DB | **Aurora PostgreSQL Serverless v2** | Schema-per-service on a shared cluster; auto-pause saves cost | Auto-pause causes cold-start latency (~5s); serverless v2 minimum 0.5 ACU still costs money when idle |
| Message broker | **Kafka (MSK)** | At-least-once delivery, independent consumer groups, replayable events, fan-out to multiple services | Operationally heavy for low traffic; Redpanda used locally (Kafka-compatible, lighter) |
| Cache | **Redis** | Shared instance for timelines, denormalised snapshots, trending, rate-limit counters | Single point of failure; mitigated in prod with ElastiCache cluster mode |
| Search | **OpenSearch 3.5** | BM25 full-text + kNN vector in one index, horizontally scalable | More operationally complex than PostgreSQL tsvector; separate index sync via Kafka required |
| Embeddings | **OpenAI `text-embedding-3-small` (256-dim)** | No separate embedding service to run; swappable via env vars | External API dependency; costs money per call; rate limits apply; requires fallback to keyword search |
| Media | **S3 + presigned URLs** | Service never handles binary data; client uploads directly to S3 | Client needs two round trips (presign + upload); S3 CORS must be configured correctly |
| Internal transport | **gRPC** | Strongly typed contracts, faster than JSON/HTTP, proto as schema | Extra build step (buf/protoc); requires proto files to stay in sync |
| Frontend | **Next.js 16.2 App Router** | Server Components + SSR for SEO and initial load; streaming; `next lint` removed in 16.2 | Biome replaces ESLint; App Router conventions differ significantly from Pages Router |

---

## Services & API Contracts

All services share these conventions:
- All public routes prefixed `/v1/`
- `/healthz` unversioned — liveness probes must never break on API version bumps
- Auth required unless marked **public**
- Timestamps: RFC 3339 (`2026-04-28T10:00:00Z`)
- Nullable fields always present in response, never omitted — `null` not absent
- Empty arrays: `[]` not `null`
- Pagination cursors: `null` on last page

---

### user-service `http://user-service:8080`

Owns user profiles and the social graph. Never issues or validates tokens.

#### `GET /v1/users/:id`
Returns a user profile. `email` is only non-null when the viewer is the owner.

**Response `200`:**
```json
{
  "id": "550e8400-e29b-41d4-a716-446655440000",
  "email": "alice@example.com",
  "username": "alice",
  "display_name": "Alice Smith",
  "avatar_url": "https://cdn.example.com/avatars/alice.jpg",
  "header_image_url": null,
  "bio": "Building things.",
  "website_url": null,
  "location": null,
  "follower_count": 120,
  "following_count": 45,
  "is_following": false,
  "created_at": "2026-01-01T00:00:00Z",
  "updated_at": "2026-04-28T10:00:00Z"
}
```
`email` is `null` when viewer ≠ profile owner.

**Errors:** `404 NOT_FOUND`

---

#### `GET /v1/users/me`
Returns the authenticated user's full profile (same shape as `GET /v1/users/:id`,
with `email` always populated).

---

#### `PATCH /v1/users/me`
Partial update on the authenticated user's profile. Only send fields you want to
change. All fields optional. Syncs `username` to Keycloak `preferred_username` on
change (via Keycloak admin API, inside an `afterCommit` callback).

**Request:**
```json
{
  "username": "alice",
  "display_name": "Alice Smith",
  "avatar_url": "https://...",
  "header_image_url": "https://...",
  "bio": "Building things.",
  "website_url": "https://alice.dev",
  "location": "Bangkok"
}
```

| Field | Type | Constraints |
|---|---|---|
| `username` | `string?` | 3–30 chars |
| `display_name` | `string?` | max 50 chars |
| `avatar_url` | `string?` | max 500 chars |
| `header_image_url` | `string?` | max 500 chars |
| `bio` | `string?` | max 160 chars |
| `website_url` | `string?` | max 200 chars |
| `location` | `string?` | max 30 chars |

`avatar_url` and `header_image_url` are obtained by the client from media-service
presigned URLs first (`POST /v1/media/presign` → PUT → use the returned
`public_url` here).

**Response `200`:** same shape as `GET /v1/users/:id`

**Errors:** `400 VALIDATION_ERROR`, `409 CONFLICT` (username taken)

---

#### `POST /v1/users/:id/follow`
Follow a user. Idempotent — following someone you already follow is a no-op.

**Response `200`:**
```json
{ "following": true }
```

**Errors:** `404 NOT_FOUND` (target user doesn't exist)

---

#### `DELETE /v1/users/:id/follow`
Unfollow. Idempotent.

**Response `200`:**
```json
{ "following": false }
```

---

#### `GET /v1/users/:id/followers?limit=20&cursor=`
Paginated followers list. Returns slim `userListItem` shape
(`id`, `username`, `display_name`, `avatar_url`, `bio`, `follower_count`,
`is_following`).

#### `GET /v1/users/:id/following?limit=20&cursor=`
Paginated followees list (same shape).

#### `GET /v1/users/suggestions?limit=20`
Lightweight "who to follow" suggestions for the authenticated viewer. Unpaginated.

---

#### `POST /internal/provision` — internal only (SERVICE_TOKEN)
Called by the Keycloak Required Action SPI on first login. Creates the DB row from the Keycloak `sub` UUID. Not exposed through Kong.

**Request:**
```json
{
  "keycloak_sub": "550e8400-e29b-41d4-a716-446655440000",
  "email": "alice@example.com",
  "display_name": "Alice Smith"
}
```

**Response `201`:** user object. Idempotent — returns `200` if row already exists.

---

#### gRPC — `UserInternal` (port 9090)

```
GetFollowerIDs(user_id)              → follower_ids[]               feed-service fan-out
GetFollowingIDs(user_id)             → following_ids[]              feed-service celeb merge at read time
GetFollowState(viewer_id, target_ids[]) → map<target_id, is_following>  search-service user-search enrichment
GetUserByID(user_id)                 → user snapshot                available but unused for author enrichment (Redis preferred)
```

Follower / following **counts** are NOT served via gRPC — they live in
`user:counts:{id}` (written by user-service on every follow/unfollow) and are
read directly from Redis by feed-service and search-service.

All gRPC calls require a `Bearer <SERVICE_TOKEN>` in the `authorization`
metadata; the interceptor rejects unauthenticated calls.

---

### tweet-service `http://tweet-service:8080`

Owns tweet storage, likes, retweets, replies, and hashtag extraction. Publishes all write events via Transactional Outbox. Maintains `tweet:counts:{id}` (HINCRBY on every like/retweet/reply mutation) and is the sole writer of `user:snapshot:{id}` via `RunUserSnapshotConsumer`, enabling author enrichment without gRPC.

#### Tweet object (shared response shape)

```json
{
  "id": "tw_01HXYZ",
  "type": "tweet",
  "author": {
    "id": "550e8400-...",
    "username": "alice",
    "display_name": "Alice Smith",
    "avatar_url": "https://..."
  },
  "body": "Hello #golang",
  "reply_to_id": null,
  "media_id": null,
  "media_url": null,
  "like_count": 5,
  "retweet_count": 2,
  "reply_count": 1,
  "hashtags": ["golang"],
  "is_liked": true,
  "is_retweeted": false,
  "created_at": "2026-04-28T10:00:00Z"
}
```

`type` is `"reply"` when `reply_to_id != null`, otherwise `"tweet"`. Retweets are
not represented as tweets — they live in a separate `retweets` table and surface
on feeds as `{ kind: "retweet", tweet, retweet }` wrappers (see
[Retweet Refactor](./RETWEET_REFACTOR.md) for the full contract).

`author` fields come from Redis `user:snapshot:{author_id}`. On cache miss the author sub-object has empty strings for username/display_name and `null` for avatar_url — no gRPC fallback.

`is_liked` / `is_retweeted` on direct reads (`GET /v1/tweets/:id`) are resolved via EXISTS subqueries in the same SQL statement. In the feed, they come from a batched `GetInteractions` gRPC call per page.

---

#### `POST /v1/tweets`

**Request:**
```json
{
  "body": "Hello #golang",
  "media_id": "med_01HXYZ",
  "media_url": "https://cdn.example.com/media/med_01HXYZ.jpg"
}
```

| Field | Required | Constraints |
|---|---|---|
| `body` | no* | max 280 chars |
| `media_id` | no* | max 50 chars |
| `media_url` | no | max 500 chars |

\* At least one of `body` or `media_id` must be non-empty — the service returns `400 BAD_REQUEST` if both are absent.

**Response `201`:** tweet object

**Side effects:** `tweet.created` published via Outbox → feed-service fans out, search-service indexes

---

#### `GET /v1/tweets/:id`

**Response `200`:** tweet object with `is_liked` / `is_retweeted` for the authenticated viewer

**Errors:** `404 NOT_FOUND`

---

#### `DELETE /v1/tweets/:id`

**Response `204`**

**Errors:** `403 FORBIDDEN` (not your tweet), `404 NOT_FOUND`

**Side effects:** `tweet.deleted` via Outbox → feed-service removes from Redis, search-service removes from index

---

#### `POST /v1/tweets/:id/like`

Idempotent — liking a tweet you already liked is a no-op (ON CONFLICT DO NOTHING).

**Response `200`:**
```json
{ "liked": true }
```

**Side effects:** `tweet.liked` via Outbox → notification-service notifies tweet author; feed-service bumps the liker's `user_interests` (hashtags) + `user_affinity` (author)

---

#### `DELETE /v1/tweets/:id/like`

**Response `200`:**
```json
{ "liked": false }
```

---

#### `POST /v1/tweets/:id/retweet`

Inserts a row into the dedicated `retweets` reference table (no copy of the
original tweet body). Deduplication via `UNIQUE (retweeter_id, original_tweet_id)`
— second attempt returns `409`.

**Response `201`:**
```json
{
  "id": "rt_01HXYZ",
  "retweeter": { "id": "...", "username": "bob", "display_name": "Bob", "avatar_url": null },
  "created_at": "2026-04-28T10:05:00Z"
}
```

**Errors:** `403 FORBIDDEN` (cannot retweet own tweet), `404 NOT_FOUND`, `409 CONFLICT` (already retweeted)

**Side effects:** `tweet.retweeted` via Outbox → feed-service writes a thin retweet entry onto follower feed lists, notification-service notifies original author

---

#### `DELETE /v1/tweets/:id/retweet`

Removes the caller's retweet row for `:id`. Publishes `tweet.unretweeted` so
feed-service scrubs the entry from followers' feed lists and notification-service
deletes the corresponding notification.

**Response `200`:**
```json
{ "retweeted": false }
```

---

#### `POST /v1/tweets/:id/reply`

Creates a new tweet with `reply_to_id` set. Replies do **not** fan-out to the replier's followers.

**Request:** same shape as `POST /v1/tweets`

**Response `201`:** tweet object with `type: "reply"`, `reply_to_id` populated

**Side effects:** `tweet.replied` via Outbox → notification-service notifies parent tweet's author

---

#### `GET /v1/tweets/:id/replies?limit=20&cursor=`

Paginated direct replies (oldest-first by tweet ID). Nested replies are supported — each reply's `reply_to_id` points to its direct parent.

**Response `200`:**
```json
{
  "tweets": [ /* tweet objects */ ],
  "next_cursor": "tw_01HABC"
}
```

`next_cursor` is `null` on the last page. Pass it as `?cursor=` on the next request.

---

#### `GET /v1/users/:id/tweets?filter=&limit=20&cursor=`

Paginated profile timeline (newest-first). `filter` selects the tab:

| `filter` | Contents |
|----------|----------|
| (absent) | Mixed: original posts + retweets authored by the user |
| `replies` | Replies authored by the user |
| `media`   | Original posts/replies with `media_id IS NOT NULL` (served by the partial index `idx_tweets_author_media`) |

All tabs return the same `{ items: [feedItem], next_cursor }` discriminated-union shape.
Replies, media, and likes items always have `kind: "tweet"` and `retweet: null` since
those tabs never surface retweets. The posts tab can have `kind: "retweet"` items.

---

#### `GET /v1/users/:id/likes?limit=20&cursor=`

Paginated list of tweets the user has liked, ordered by `likes.created_at DESC`.

**Authorisation:** owner-only — `403 FORBIDDEN` if the viewer is not the path
user. Likes are private.

**Response `200`:** same `{ items: [feedItem], next_cursor }` shape as all profile timeline tabs.

---

#### gRPC — `TweetInternal` (port 9090)

```
GetTweet(tweet_id, viewer_id?)                  → tweet with is_liked/is_retweeted for viewer
GetTweetsByAuthor(author_id, limit)             → recent tweets by author (celeb merge)
GetRecentPopularTweets(limit)                   → recent tweets sorted by engagement (recommended feed candidates)
GetInteractions(viewer_id, tweet_ids[])         → map<tweet_id, {is_liked, is_retweeted}>
```

`GetInteractions` is the key performance optimisation for feeds: instead of N per-tweet queries, one batched call resolves viewer-specific flags for a whole page.

---

### feed-service `http://feed-service:8080`

Consumes Kafka events, manages Redis timelines, serves feeds and trending. No
database — Redis only. No gRPC server.

#### Feed item (discriminated union)

```json
{
  "kind": "tweet",
  "tweet": {
    "id": "tw_01HXYZ",
    "type": "tweet",
    "author": { "id": "...", "username": "alice", "display_name": "Alice", "avatar_url": "https://..." },
    "body": "Hello #golang",
    "reply_to_id": null,
    "media_id": null,
    "media_url": null,
    "like_count": 5,
    "retweet_count": 2,
    "reply_count": 1,
    "hashtags": ["golang"],
    "is_liked": true,
    "is_retweeted": false,
    "created_at": "2026-04-28T10:00:00Z"
  },
  "retweet": null
}
```

For a retweet, `kind` is `"retweet"`, `tweet` is the canonical original (same
shape), and `retweet` carries the thin retweet record:

```json
{
  "kind": "retweet",
  "tweet": { /* original tweet */ },
  "retweet": {
    "id": "rt_01HXYZ",
    "retweeter": { "id": "...", "username": "bob", "display_name": "Bob", "avatar_url": null },
    "created_at": "2026-04-28T10:05:00Z"
  }
}
```

The tweet payload is byte-identical to one served by tweet-service — no
top-level `author_id`, no `omitempty`, every field is always present. Wire-level
types live in `internal/server/types.go`; mappers (`newFeedItem`, etc.) convert
the internal `feed.FeedItem` view into the wire shape.

Enrichment per page:
- Static fields (`body`, `author_*`, `media_url`, etc.) from `tweet:snapshot:{id}`
- Counts (`like_count`, `retweet_count`, `reply_count`) from `tweet:counts:{id}`
- Interaction flags (`is_liked`, `is_retweeted`) from one batched `GetInteractions` gRPC call per page

---

#### `GET /v1/feed/following?limit=20&cursor=`

Returns the authenticated user's following feed. See [Fan-out & Timeline](#fan-out--timeline) for the full read algorithm.

**Response `200`:**
```json
{
  "items": [ /* feed items */ ],
  "next_cursor": "tw_01HABC"
}
```

---

#### `GET /v1/feed/recommended?limit=20&cursor=` 

Returns personalised recommended tweets scored by the viewer's interest + affinity signals. Cached per-user for 15 minutes. See [Recommended Feed](#recommended-feed) for scoring.

**Response `200`:** same shape as following feed

---

#### `GET /v1/feed/trending?limit=10` — **public, no auth**

Returns top hashtags by tweet count. Maximum limit: 50.

**Response `200`:**
```json
{
  "trending": [
    { "tag": "golang", "score": 142 },
    { "tag": "nextjs", "score": 87 }
  ]
}
```

---

### notification-service `http://notification-service:8080`

Consumes Kafka events, persists notifications in PostgreSQL, streams them via SSE. Never publishes events.

#### Notification object

```json
{
  "id": "ntf_01HXYZ",
  "type": "like",
  "actor_id": "550e8400-...",
  "actor_username": "bob",
  "actor_display_name": "Bob Jones",
  "actor_avatar_url": "https://...",
  "tweet_id": "tw_01HABC",
  "tweet_preview": "Hello #golang",
  "read_at": null,
  "created_at": "2026-04-28T10:00:00Z"
}
```

`type` is one of: `like`, `retweet`, `reply`, `follow`.  
`tweet_id` and `tweet_preview` are `null` for `follow` notifications.  
`actor_*` fields enriched from `user:snapshot:{actor_id}` at read time.  
`tweet_preview` enriched from `tweet:snapshot:{tweet_id}` at read time.

---

#### `GET /v1/notifications?limit=20&cursor=`

Paginated notification history, newest-first. Cursor = last `ntf_` ID seen.

**Response `200`:**
```json
{
  "notifications": [ /* notification objects */ ],
  "next_cursor": "ntf_01HABC"
}
```

---

#### `PATCH /v1/notifications/read`

Mark **all** unread notifications as read.

**Response `200`:**
```json
{ "read": true }
```

---

#### `PATCH /v1/notifications/:id/read`

Mark a single notification as read (idempotent).

**Response `200`:** notification object with `read_at` set

**Errors:** `403 FORBIDDEN` (not your notification), `404 NOT_FOUND`

---

#### `GET /v1/notifications/stream`

Server-Sent Events stream. The browser `EventSource` auto-reconnects on disconnect — do not close on error in client code.

**SSE events:**

```
# Sent immediately on connect — client uses this to sync badge without a separate request
event: count
data: {"count": 3}

# Sent for each new notification
event: notification
data: {"id":"ntf_01HXYZ","type":"like","actor_id":"...","actor_username":"bob",...}

# Sent every 30 seconds to keep connection alive through proxies
: heartbeat
```

The service maintains an in-memory subscriber map (`map[userID][]chan Notification`). Multiple tabs per user are supported. Publish to a slow consumer is non-blocking — the event is dropped rather than blocking the Kafka consumer.

---

### media-service `http://media-service:8080`

Generates presigned S3 URLs. Never handles binary data.

#### `POST /v1/media/presign`

**Request:**
```json
{ "content_type": "image/jpeg" }
```

**Response `200`:**
```json
{
  "media_id": "med_01HXYZ",
  "upload_url": "https://s3.amazonaws.com/bucket/med_01HXYZ?X-Amz-Signature=...",
  "public_url": "https://cdn.example.com/media/med_01HXYZ",
  "expires_at": "2026-04-28T10:15:00Z"
}
```

**Client flow:**
1. `POST /v1/media/presign` → get `upload_url` + `public_url`
2. `PUT {upload_url}` with binary data (direct to S3, Content-Type must match)
3. Include `media_id` + `public_url` in the tweet/reply body request

The `public_url` is stored on the tweet row. tweet-service never contacts media-service — it just stores whatever the client provides.

---

### search-service `http://search-service:8080`

Keyword and semantic search over tweets and users. OpenSearch is the index; Redis
and tweet-service/user-service gRPC supply the read-time enrichment so search
results render with the same shape as feed / profile / tweet endpoints. Synced
via Kafka consumer group `search-service-cg`.

#### `GET /v1/search/tweets?q=&mode=&limit=20&cursor=`

| Param | Values | Default |
|---|---|---|
| `q` | query string | required |
| `mode` | `keyword`, `semantic`, `hybrid` | `hybrid` |
| `limit` | 1–100 | 20 |
| `cursor` | opaque string from previous response | — |

**Response `200`:**
```json
{
  "tweets": [
    {
      "id": "tw_01HXYZ",
      "author": {
        "id": "550e8400-...",
        "username": "alice",
        "display_name": "Alice Smith",
        "avatar_url": "https://..."
      },
      "body": "Hello #golang",
      "hashtags": ["golang"],
      "reply_to_id": null,
      "media_id": null,
      "media_url": null,
      "like_count": 5,
      "retweet_count": 2,
      "reply_count": 1,
      "is_liked": false,
      "is_retweeted": false,
      "created_at": "2026-04-28T10:00:00Z"
    }
  ],
  "next_cursor": null
}
```

Read-time enrichment (`internal/enrich/`):
- Author snapshot — pipelined `HGETALL user:snapshot:{author_id}` (deduplicated within a page)
- Live counts — pipelined `HGETALL tweet:counts:{id}`; on miss falls back to the denormalised `like_count`/`retweet_count` stored on the OpenSearch document
- Viewer flags (`is_liked`, `is_retweeted`) — single batched `tweet-service.TweetInternal.GetInteractions` call (gobreaker-wrapped)

Graceful degradation: Redis misses render as empty author fields / zero counts.
A tweet-service gRPC failure logs and degrades to `is_liked = is_retweeted = false`
rather than failing the search.

**Fallback:** when `mode=semantic` or `mode=hybrid` and the OpenAI API is unavailable, the service falls back to keyword search and sets `X-Search-Mode: keyword-fallback` response header.

---

#### `GET /v1/search/users?q=&limit=10&cursor=`

BM25 search over `username`, `display_name`, and `bio` (user search is
keyword-only — semantic over usernames adds marginal value).

**Response `200`:**
```json
{
  "users": [
    {
      "id": "550e8400-...",
      "username": "alice",
      "display_name": "Alice Smith",
      "avatar_url": "https://...",
      "bio": "Building things.",
      "follower_count": 120,
      "is_following": false
    }
  ],
  "next_cursor": null
}
```

`avatar_url` is stored on the OpenSearch `UserDoc` (retrieval-only `keyword`,
not scored). `follower_count` is read from Redis `user:counts:{id}` via a
pipelined HGET. `is_following` is enriched per request via
`user-service.UserInternal.GetFollowState` (gobreaker-wrapped). Both degrade
to zero/false on error rather than failing the search.

---

### All services — `GET /healthz` and `GET /livez`

Two endpoints, both unauth:

- **`/livez`** — liveness. Always 200 if the process is running. Used by k8s `livenessProbe` and the docker-compose container health check. A transient backing-store flake must not restart the pod.
- **`/healthz`** — readiness. Probes the service's real dependencies (DB, Redis, OpenSearch, …) concurrently with a 1.5s timeout. Returns 200 with per-dep status when all are reachable; 503 with the same shape (and an `error` field per failed dep) when any fails. Used by k8s `readinessProbe`, the ALB target group, and the `make healthcheck` dev target — pod is taken out of rotation while deps are unreachable.

**`/healthz` 200 response:**
```json
{
  "status": "ok",
  "service": "tweet-service",
  "deps": [
    { "name": "postgres", "status": "ok", "error": "" },
    { "name": "redis",    "status": "ok", "error": "" }
  ]
}
```

**`/healthz` 503 response** (one dep down):
```json
{
  "status": "degraded",
  "service": "tweet-service",
  "deps": [
    { "name": "postgres", "status": "ok",    "error": "" },
    { "name": "redis",    "status": "error", "error": "dial tcp 10.0.0.4:6379: connect: connection refused" }
  ]
}
```

**`/livez` 200 response:** `{ "status": "ok", "service": "tweet-service", "deps": [] }`

---

## Error Handling

All errors follow a single consistent shape across every service:

```json
{
  "code": "NOT_FOUND",
  "message": "not found",
  "fields": null,
  "request_id": "54370939d17ac07a311332361365d121"
}
```

`fields` is only present on `VALIDATION_ERROR` — maps snake_case field names to human error messages:

```json
{
  "code": "VALIDATION_ERROR",
  "message": "validation failed",
  "fields": {
    "body": "maximum length is 280",
    "username": "minimum length is 3"
  },
  "request_id": "..."
}
```

**Standard error codes:**

| HTTP | Code | Meaning |
|------|------|---------|
| 400 | `BAD_REQUEST` | Malformed request |
| 400 | `VALIDATION_ERROR` | Field validation failed (with `fields` map) |
| 401 | `UNAUTHORIZED` | Missing or invalid token |
| 403 | `FORBIDDEN` | Authenticated but not allowed |
| 404 | `NOT_FOUND` | Resource doesn't exist |
| 409 | `CONFLICT` | Duplicate (e.g. already retweeted, username taken) |
| 500 | `INTERNAL_ERROR` | Unhandled server error — always logged |

**In services**, errors propagate as sentinel values from `shared/httperr`:
```go
// Service returns — never raw errors
return httperr.ErrNotFound
return httperr.ErrForbidden
return httperr.New(http.StatusConflict, "ALREADY_RETWEETED", "already retweeted")

// Handler converts — never write your own JSON error
httperr.Handle(c, err, s.log)   // maps sentinels → status + JSON; unknown errors → 500 + log
httperr.Validation(c, err)       // parses go-playground/validator errors into field map
```

DB errors are mapped at the store layer: `pgxutil.MapErr(err)` converts `pgx.ErrNoRows` → `ErrNotFound`, unique constraint violations → conflict errors.

---

## Service Dependency Map

```
                  ┌──────────────────────────────────────────────────────┐
                  │                   KAFKA                               │
                  │  tweet.created / tweet.deleted / tweet.liked          │
                  │  tweet.retweeted / tweet.unretweeted / tweet.replied  │
                  │  user.created / user.followed / user.updated          │
                  │  media.uploaded                                       │
                  └──┬─────────────────────┬────────────────────┬────────┘
              consume │              consume│             consume│
    ┌──────────────────▼──┐  ┌─────────────▼─────────┐  ┌──────▼──────────────┐
    │    feed-service     │  │  notification-service │  │   search-service    │
    │  (fan-out, redis,   │  │  (SSE hub + DB write) │  │  (OpenSearch index  │
    │   recommended)      │  └───────────────────────┘  │   + read enrichment)│
    └──────┬──────────────┘             │               └─────────┬───────────┘
           │ gRPC                       │ Redis read              │ gRPC + Redis
     ┌─────▼──────────┐   ┌─────────────▼──────────────────┐      │
     │  user-service  │   │  tweet-service                 │◄─────┘
     │                │   │  + RunUserSnapshotConsumer     │
     └────────────────┘   │    (sole writer user:snapshot, │
                          │     HINCRBY tweet:counts)      │
                          └────────────────────────────────┘
                                       │
                                       ▼
                              ElastiCache Redis
```

**Call summary:**

| Caller | Target | Transport | Why |
|--------|--------|-----------|-----|
| tweet-service | Redis | `HGETALL pipeline` | Author enrichment on every tweet read |
| tweet-service | Redis | `HINCRBY tweet:counts:{id}` | Authoritative like/retweet/reply counts |
| tweet-service | Kafka (consumer) | `user.created`, `user.updated` | Sole writer of `user:snapshot:{id}` |
| tweet-service | Keycloak admin | HTTPS | `username` sync on profile update — fire-and-forget post-commit |
| feed-service | user-service | gRPC | Fan-out: follower IDs + count; read-time celeb merge |
| feed-service | tweet-service | gRPC | Celeb-merge tweets; recommended candidates; viewer interaction flags |
| feed-service | Kafka (consumer) | 7 topics | Maintain feed lists, trending, affinity, embedded author fields |
| notification-service | Redis | `HGETALL` | Enrich actor + tweet preview at read time |
| notification-service | Redis Pub/Sub | publish/subscribe `notifications` | Multi-pod SSE fan-out |
| notification-service | Kafka (consumer) | 5 topics | Persist notifications; push via SSE |
| search-service | Redis | `HGETALL pipeline` | Author snapshots + live counts for search results |
| search-service | Redis | `HGET user:counts:*` | `follower_count` for user search results |
| search-service | user-service | gRPC | `GetFollowState` for `is_following` on user search |
| search-service | tweet-service | gRPC | `GetInteractions` for viewer flags on tweet search |
| search-service | OpenAI API | HTTPS | Generate 256-dim embeddings for index + query |
| search-service | Kafka (consumer) | 4 topics | Keep OpenSearch index in sync |

**Single writer for `user:snapshot:{id}`.** tweet-service's
`RunUserSnapshotConsumer` is the only process that writes the user-snapshot
hash. feed-service, notification-service, and search-service read it.
feed-service additionally consumes `user.updated` so it can refresh the
embedded author fields stored on `tweet:snapshot:*` via the
`author_tweets:{user_id}` reverse index (a separate concern from the user
snapshot itself).

---

## Data Models

Services share one PostgreSQL cluster but each runs in its own schema
(`users`, `tweet`, `notification`). No cross-schema joins — owning service is
the only writer. The outbox lives only in the schemas of services that publish
events (tweet-service, user-service); notification-service is consume-only.

### user-service (schema: `users`)

```sql
CREATE TABLE users (
    id               TEXT        PRIMARY KEY,     -- Keycloak sub UUID
    email            TEXT        UNIQUE NOT NULL,
    username         TEXT        UNIQUE,          -- NULL until onboarding complete
    display_name     TEXT,
    avatar_url       TEXT,
    header_image_url TEXT,
    bio              TEXT,
    website_url      TEXT,
    location         TEXT,
    follower_count   INTEGER     NOT NULL DEFAULT 0,   -- denormalised
    following_count  INTEGER     NOT NULL DEFAULT 0,   -- denormalised
    created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE follows (
    follower_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    followee_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at  TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (follower_id, followee_id),
    CHECK (follower_id <> followee_id)           -- cannot follow yourself
);
```

`follower_count` / `following_count` are denormalised on the `users` table and incremented/decremented in the same transaction as the follow/unfollow. This avoids a COUNT query on every profile view.

`user.id` is the Keycloak `sub` UUID — this eliminates a `keycloak_id` column and makes the `X-User-ID` header from Kong immediately usable as a DB key without a lookup.

### tweet-service (schema: `tweet`)

```sql
CREATE TABLE tweets (
    id            TEXT PRIMARY KEY,       -- tw_{ulid}
    author_id     TEXT NOT NULL,          -- no FK — cross-service boundary
    body          TEXT NOT NULL CHECK (char_length(body) <= 280),
    reply_to_id   TEXT REFERENCES tweets(id) ON DELETE SET NULL,
    media_id      TEXT,
    media_url     TEXT,
    like_count    INT NOT NULL DEFAULT 0,
    retweet_count INT NOT NULL DEFAULT 0,
    reply_count   INT NOT NULL DEFAULT 0,
    created_at    TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_tweets_author_id   ON tweets(author_id);
CREATE INDEX idx_tweets_created_at  ON tweets(created_at DESC);
CREATE INDEX idx_tweets_reply_to_id ON tweets(reply_to_id, created_at ASC);
-- Drives the profile "Media" tab — covers author_id + created_at, filtered to media.
CREATE INDEX idx_tweets_author_media ON tweets(author_id, created_at DESC)
    WHERE media_id IS NOT NULL;

CREATE TABLE retweets (
    id                TEXT PRIMARY KEY,       -- rt_{ulid}
    retweeter_id      TEXT NOT NULL,
    original_tweet_id TEXT NOT NULL REFERENCES tweets(id) ON DELETE CASCADE,
    created_at        TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (retweeter_id, original_tweet_id) -- dedup; O(1) "have I retweeted this?"
);

CREATE INDEX idx_retweets_retweeter_created ON retweets(retweeter_id, created_at DESC);
CREATE INDEX idx_retweets_original          ON retweets(original_tweet_id);

CREATE TABLE likes (
    user_id    TEXT NOT NULL,
    tweet_id   TEXT NOT NULL REFERENCES tweets(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (user_id, tweet_id)
);

CREATE INDEX idx_likes_user_created ON likes(user_id, created_at DESC, tweet_id DESC);

CREATE TABLE outbox (
    id         BIGSERIAL PRIMARY KEY,
    topic      TEXT NOT NULL,
    key        TEXT NOT NULL,
    payload    JSONB NOT NULL,
    trace_id   TEXT NOT NULL DEFAULT '',   -- W3C traceparent captured at enqueue
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    sent_at    TIMESTAMPTZ                  -- NULL = pending; set after Kafka publish
);

CREATE INDEX idx_outbox_unsent ON outbox(id) WHERE sent_at IS NULL;
```

**Design decisions:**
- **Reference-only retweets.** Retweets live in their own `retweets` table — no
  body copy, no row on `tweets`. The feed transports them as a discriminated
  `{ kind: "retweet", tweet, retweet }` wrapper. See
  [docs/RETWEET_REFACTOR.md](./RETWEET_REFACTOR.md).
- **No `type` column.** Type is derived at response time on the `tweets` row:
  `reply_to_id != null` → reply, else tweet. Retweets are not tweets.
- **`author_id` has no FK** into users — that table lives in a different schema. Referential integrity is enforced by the application.
- **Counts are denormalised** onto the tweet row and incremented in the same
  transaction as the action. The same write also issues `HINCRBY tweet:counts:{id}`
  on Redis (best-effort, post-commit) so feed/search reads see authoritative live counts
  without re-querying Postgres.

### notification-service (schema: `notification`)

```sql
CREATE TYPE notif_type AS ENUM ('like', 'retweet', 'reply', 'follow');

CREATE TABLE notifications (
    id         TEXT PRIMARY KEY,         -- ntf_{ulid}
    user_id    TEXT NOT NULL,            -- recipient
    actor_id   TEXT NOT NULL,            -- who triggered it
    type       notif_type NOT NULL,
    tweet_id   TEXT,                     -- NULL for follow notifications
    read_at    TIMESTAMPTZ,              -- NULL = unread
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX idx_notifications_user_id ON notifications(user_id, created_at DESC);

-- Idempotency table. event_id is the outbox row ID carried in the
-- X-Event-ID Kafka header; the consumer records it in the same tx as the
-- notification insert so redelivery cannot duplicate.
CREATE TABLE processed_events (
    event_id     BIGINT      PRIMARY KEY,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
```

A background goroutine (`RunDedupPrune`) trims `processed_events` rows older
than 7 days every 6 hours — beyond that horizon no broker will redeliver.

### search-service (OpenSearch indices — no SQL)

**`tweets` index:**
```json
{
  "settings": { "knn": true },
  "mappings": {
    "properties": {
      "id":            { "type": "keyword" },
      "author_id":     { "type": "keyword" },
      "body":          { "type": "text", "analyzer": "english" },
      "hashtags":      { "type": "keyword" },
      "like_count":    { "type": "integer" },
      "retweet_count": { "type": "integer" },
      "reply_to_id":   { "type": "keyword", "index": false },
      "media_id":      { "type": "keyword", "index": false },
      "media_url":     { "type": "keyword", "index": false },
      "created_at":    { "type": "date" },
      "vector": {
        "type": "knn_vector", "dimension": 256,
        "method": { "name": "hnsw", "space_type": "cosinesimil", "engine": "faiss",
                    "parameters": { "ef_construction": 128, "m": 16 } }
      }
    }
  }
}
```

`like_count` / `retweet_count` here are snapshot values for ranking; the read
path re-fetches live counts from `tweet:counts:{id}` Redis (the document value
is used as a fallback on cache miss). `reply_to_id` / `media_id` / `media_url`
are retrieval-only — stored to render search results without a second hop, not
scored.

**`users` index:**
```json
{
  "settings": { "knn": true },
  "mappings": {
    "properties": {
      "id":           { "type": "keyword" },
      "username":     { "type": "text", "analyzer": "standard",
                        "fields": { "keyword": { "type": "keyword" } } },
      "display_name": { "type": "text", "analyzer": "standard" },
      "bio":          { "type": "text", "analyzer": "english" },
      "avatar_url":   { "type": "keyword", "index": false },
      "updated_at":   { "type": "date" },
      "vector":       { "type": "knn_vector", "dimension": 256, "method": { /* hnsw */ } }
    }
  }
}
```

`avatar_url` is retrieval-only. `follower_count` and `is_following` are NOT
indexed — they are enriched per request from user-service gRPC.

Indices are created at startup (idempotent). If OpenAI embedding fails at index time, the document is stored without a vector — it is still searchable by keyword but excluded from kNN results.

---

## Kafka Events

All events serialised as JSON. Located in `apps/shared/events/`.

```go
type TweetCreatedEvent struct {
    TweetID   string    `json:"tweet_id"`
    AuthorID  string    `json:"author_id"`
    Body      string    `json:"body"`
    Hashtags  []string  `json:"hashtags"`
    ReplyToID string    `json:"reply_to_id,omitempty"`
    MediaID   string    `json:"media_id,omitempty"`
    MediaURL  string    `json:"media_url,omitempty"`
    CreatedAt time.Time `json:"created_at"`
}

type TweetDeletedEvent     { TweetID, AuthorID string }
type TweetLikedEvent       { TweetID, LikerID, AuthorID string; Hashtags []string }
type TweetRetweetedEvent   { RetweetID, OriginalTweetID, OriginalAuthorID, RetweeterID string; CreatedAt time.Time }
type TweetUnretweetedEvent { RetweetID, OriginalTweetID, RetweeterID string }
type TweetRepliedEvent     { ReplyID, ParentTweetID, ParentAuthorID, ReplierID string }
type UserCreatedEvent      { UserID, Username, DisplayName, Bio, AvatarURL string; CreatedAt time.Time }
type UserFollowedEvent     { FollowerID, FolloweeID string; CreatedAt time.Time }
type UserUpdatedEvent      { UserID, Username, DisplayName, Bio, AvatarURL string }
type MediaUploadedEvent    { MediaID, UploaderID, S3Key string }
```

**Topics:**

| Topic | Publisher | Key | Consumers |
|-------|-----------|-----|-----------|
| `tweet.created`     | tweet-service | `author_id`        | feed-service, search-service |
| `tweet.deleted`     | tweet-service | `author_id`        | feed-service, search-service |
| `tweet.liked`       | tweet-service | `tweet_id`         | feed-service (affinity signals), notification-service |
| `tweet.retweeted`   | tweet-service | `original_tweet_id`| feed-service, notification-service |
| `tweet.unretweeted` | tweet-service | `original_tweet_id`| feed-service, notification-service |
| `tweet.replied`     | tweet-service | `parent_tweet_id`  | notification-service |
| `user.created`      | user-service  | `user_id`          | tweet-service (sole `user:snapshot` writer), search-service |
| `user.followed`     | user-service  | `followee_id`      | feed-service (affinity), notification-service |
| `user.updated`      | user-service  | `user_id`          | tweet-service (`user:snapshot`), feed-service (refresh embedded author on `tweet:snapshot:*`), search-service |
| `media.uploaded`    | (reserved)    | `media_id`         | Topic + event struct exist for future orphan-upload GC; not yet published. |

Partition key choice is intentional: `tweet.created` is keyed by `author_id` so all tweets from the same author land in the same partition, preserving order for fan-out. `tweet.liked` keyed by `tweet_id` so all likes for a tweet land together.

Retention: 7 days. Each consumer group tracks offsets independently — adding a new consumer to an existing topic doesn't affect others.

### Consumer-side resilience

Each consumer group has a dedicated dead-letter topic for poison messages
(payloads that fail JSON deserialization). Poison messages are routed to DLQ
then committed through so the partition keeps moving; the original payload is
preserved with provenance headers (`X-DLQ-Source-Topic`, `X-DLQ-Source-Partition`,
`X-DLQ-Source-Offset`, `X-DLQ-Error`) for inspection and replay via
`cmd/dlq-replay`.

| DLQ topic | Owner |
|-----------|-------|
| `feed-service-cg.dlq`         | feed-service |
| `notification-service-cg.dlq` | notification-service |
| `search-service-cg.dlq`       | search-service |

**Idempotency.** The outbox flusher stamps every Kafka message with an
`X-Event-ID` header carrying the originating outbox row ID. Consumers use this
to deduplicate on redelivery:
- feed-service / search-service: `SETNX feed:dedup:{event_id}` (24h TTL) in Redis
- notification-service: `INSERT INTO processed_events(event_id)` in the same tx
  as the notification write (`RunDedupPrune` removes rows older than 7 days)

---

## Redis Key Design

Redis is the shared data layer for everything that needs low-latency reads without a DB query. Keycloak owns session state (refresh tokens, OAuth state) in a separate store — those keys are not here.

```
# ── User snapshots ──────────────────────────────────────────────────────────
user:snapshot:{user_id}
  HASH  username, display_name, avatar_url
  TTL:  none (overwritten on user.updated)
  Written by: tweet-service (RunUserSnapshotConsumer — sole writer)
  Read by:    tweet-service (author enrichment), feed-service (embedding into
              tweet:snapshot at tweet.created time + via tweet:snapshot fields),
              notification-service (actor enrichment),
              search-service (tweet-search author enrichment)

user:counts:{user_id}
  HASH  follower_count, following_count
  TTL:  none (overwritten on follow/unfollow; reconciled from Postgres on
              user-service boot via RefreshAllCounts)
  Written by: user-service (sole writer — HINCRBY after each follow/unfollow
              tx commits; HSET 0/0 on Provision)
  Read by:    feed-service (celebrity-threshold check during fan-out),
              search-service (user-search follower_count enrichment)

# ── Tweet snapshots ──────────────────────────────────────────────────────────
tweet:snapshot:{tweet_id}
  HASH  body, author_id, type, reply_to_id, media_id, media_url,
        hashtags, created_at, author_username, author_display_name, author_avatar_url
  TTL:  none
  Written by: feed-service on tweet.created (embedded author copied from user:snapshot)
  Read by:    feed-service (feed enrichment), notification-service (tweet_preview)
  NOTE: retweets are reference-only and do NOT get a tweet:snapshot — the
        original tweet's snapshot covers them.

tweet:counts:{tweet_id}
  HASH  like_count, retweet_count, reply_count
  TTL:  none
  Written by: tweet-service via HINCRBY (post-commit, best-effort) on every
              like/retweet/reply mutation
  Read by:    feed-service, search-service (feed/search enrichment)

author_tweets:{user_id}
  SET   tweet_id members
  TTL:  none
  Used to propagate profile changes: when user.updated fires, feed-service iterates
  this set and rewrites author fields in each tweet:snapshot

# ── Following feed ───────────────────────────────────────────────────────────
following:{user_id}
  LIST  feed entries, newest-first, capped at 800 (LTRIM)
  TTL:  7 days (reset on each LPUSH)
  Written by: feed-service on tweet.created / tweet.retweeted / tweet.unretweeted
  Entry encoding (string, "|"-delimited):
    Original tweet  →  "T|<tweet_id>"
    Retweet         →  "R|<rt_id>|<original_tweet_id>|<retweeter_id>|<epoch>"
  See consumer.ParseFeedEntry / EncodeTweetEntry / EncodeRetweetEntry.

following_cache:{user_id}
  STRING  JSON-encoded assembled first page (post-celeb-merge, post-enrichment)
  TTL:    60 seconds
  Avoids re-scanning the LIST + re-merging celeb tweets on repeated first-page loads

# ── Dedup ────────────────────────────────────────────────────────────────────
feed:dedup:{event_id}
  STRING  "1"
  TTL:    24 hours
  Set by feed-service after a Kafka event is processed; the consumer's first
  check on redelivery is EXISTS on this key.

# ── Celebrity flag ───────────────────────────────────────────────────────────
celeb:{user_id}
  STRING  "1"
  TTL:    1 hour
  Set when follower_count >= 1000. Author with this flag has fan-out skipped at write
  time; their tweets are merged lazily at feed read time via GetTweetsByAuthor gRPC.

# ── Trending ─────────────────────────────────────────────────────────────────
trending:minute:{minute_epoch}
  SORTED SET  member=tag, score=count within that minute
  TTL:        70 minutes (auto-decay — older buckets drop out of the window)

trending:window:{minute_epoch}
  SORTED SET  ZUNIONSTORE destination over the last 60 trending:minute:* buckets;
              read-side cache, atomically overwritten by concurrent callers.
  TTL:        5 minutes

# ── Recommended feed ─────────────────────────────────────────────────────────
recommended:{user_id}
  LIST  tweet_ids, highest-scored first
  TTL:  controlled by recommended_ts key

recommended_ts:{user_id}
  STRING  "1"
  TTL:    15 minutes
  Presence = cache is fresh. Absence = trigger rebuild on next request.

# ── Personalisation signals ──────────────────────────────────────────────────
user_interests:{user_id}
  SORTED SET  member=hashtag, score=affinity
  TTL:        30 days
  Incremented when user likes/retweets a tweet with that hashtag

user_affinity:{user_id}
  SORTED SET  member=author_id, score=affinity
  TTL:        30 days
  Incremented when user interacts with tweets from that author
  (follow = 5×, retweet = 3×, like = 1×)

# ── SSE Pub/Sub ──────────────────────────────────────────────────────────────
notifications  (channel, not a key)
  Redis Pub/Sub channel — notification-service publishes here when a new
  notification is created; every pod subscribed via RunPubSub fans out
  locally to in-memory SSE subscribers. Enables multi-pod SSE without sticky
  sessions.
```

---

## Authentication & Gateway

### Keycloak (Identity Provider)

All authentication is delegated to **Keycloak 26**. user-service never issues or validates tokens — it only manages profiles.

**Why Keycloak instead of managed auth?** We want Google OIDC federation, a custom provisioning step (creating the DB row before the token is issued), and full control over token lifetimes. Keycloak's Required Action SPI makes the provisioning step native to the login flow.

**Local dev:** `start-dev` mode, PostgreSQL-backed. Realm config imported from
`local/keycloak/realm-twitter.json`, which is generated from
`realm-twitter.template.json` by `make generate-realm` using `.env` values
(`GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`). Direct access grants enabled for
API testing via Postman.

**Production:** `start` mode. TLS via cert-manager. Direct access grants disabled — Google OIDC only.

user-service reaches the Keycloak admin API (to keep `preferred_username` in
sync on profile edits) via a dedicated confidential client. The credentials are
configured per environment as `KEYCLOAK_ADMIN_CLIENT_ID` and
`KEYCLOAK_ADMIN_CLIENT_SECRET` env vars on user-service.

### User Provisioning (Required Action SPI)

The chicken-and-egg problem: Keycloak creates a user account, but user-service needs to create the DB row before the JWT is issued (so the very first API call works).

**Solution:** A Keycloak Required Action SPI (`local/keycloak-spi/`) that fires synchronously during the `first-broker-login` flow:

```
1. User authenticates via Google OIDC
2. Keycloak creates the Keycloak account and generates a sub UUID
3. Before issuing any token, Keycloak evaluates Required Actions
4. PROVISION_USER action fires:
   → calls POST /internal/provision with {sub, email, display_name}
   → user-service inserts the DB row + publishes user.created via Outbox
5. SPI marks user as provisioned, calls context.success()
6. Keycloak issues the access token
```

`/internal/provision` is idempotent — safe to call twice (returns `200` if row exists). After first login, the frontend checks `username == null` in the profile response and redirects to `/onboarding`.

### JWT Claims

```json
{
  "iss": "http://keycloak:8080/realms/twitter",
  "sub": "550e8400-e29b-41d4-a716-446655440000",
  "email": "alice@example.com",
  "preferred_username": "alice",
  "exp": 1712000900
}
```

`sub` is used directly as `users.id`. No mapping layer needed.

### Kong Gateway

Kong is the single internet-facing entry point. Every protected route runs JWT validation plus a Lua pre-function:

```
Request → [jwt plugin validates RS256] → [lua_keycloak] → Service
```

**`lua_keycloak`:** Decodes the JWT payload (already verified by the jwt plugin), extracts `sub`, `email`, `preferred_username`, injects `X-User-ID`, `X-User-Email`, `X-User-Username` headers.

Services read Kong-injected headers directly via `auth.HeadersMiddleware()` — no JWT parsing in service code. Account suspension is handled via Keycloak (short access-token TTL + admin-API session logout); there is no live blocklist on the request path.

**Two Kong 3.x-specific decisions:**

1. **No `priority` field on routes.** Removed in Kong 3.x declarative config. Regex paths (`~/v1/users/[^/]+/tweets$`) automatically beat prefix paths (`/v1/users/`).

2. **`kong.service.request` is write-only in Kong 3.x.** The sub extracted in `lua_keycloak` is injected as an HTTP header rather than read back inside Kong.

**Production difference:** The jwt plugin points at Keycloak's JWKS endpoint (`/realms/twitter/protocol/openid-connect/certs`), not a hardcoded public key. Key rotation in Keycloak propagates to Kong automatically.

### Frontend Auth Flow

Next.js implements OAuth 2.0 Authorization Code flow against Keycloak. No third-party auth library.

**Cookies:**

| Cookie | `httpOnly` | Contents | Lifetime |
|--------|-----------|---------|---------|
| `access_token` | yes | Keycloak RS256 JWT | 900 s |
| `refresh_token` | yes | Keycloak refresh token | 36 000 s |
| `id_token` | yes | OIDC ID token (for logout only) | 900 s |

**Silent refresh (`src/middleware.ts`):** runs on every non-auth request; if `exp − now ≤ 60s`, exchanges the refresh token for a new pair; redirects to `/login` if refresh fails.

**Logout:** clears cookies and redirects to Keycloak's end-session endpoint with `id_token_hint` + `post_logout_redirect_uri`. This terminates the Keycloak SSO session so the next login forces a fresh Google auth.

**Login sequence (abbreviated):**
```
Browser → GET /login → redirect to Keycloak with kc_idp_hint=google
→ Google consent → code returned to Keycloak
→ Keycloak: PROVISION_USER SPI fires → POST /internal/provision
→ code redirect to GET /api/auth/callback
→ Next.js: POST /token (code exchange) → set httpOnly cookies
→ middleware: username==null? → redirect /onboarding
→ onboarding: PATCH /v1/users/me → redirect /home
```

---

## Core Patterns

### Transactional Outbox

tweet-service and user-service never write to Kafka directly. Every mutation:

1. `INSERT` the primary row (tweet, follow, etc.)
2. `INSERT` into `outbox(topic, key, payload)` in the **same DB transaction**
3. Background goroutine polls `SELECT … WHERE sent_at IS NULL`, publishes to Kafka, marks `sent_at = now()`

**Why:** Eliminates dual-write. If Kafka is down, the event stays in the outbox and is retried. The DB transaction is the unit of atomicity — there's no window where a tweet exists without a corresponding event.

**Trade-off:** Events are not published instantly — there's a polling delay (≈1s). At-least-once: if the service crashes after Kafka publish but before `sent_at` is set, the event is re-published. Consumers use `ON CONFLICT DO NOTHING` to handle duplicates.

### Fan-out & Timeline

**Write path (on `tweet.created` / `tweet.retweeted`):**

```
encode entry:
    original tweet  →  "T|<tweet_id>"
    retweet         →  "R|<rt_id>|<original_tweet_id>|<retweeter_id>|<epoch>"

follower_count = HGET user:counts:{author_id} follower_count

if follower_count < 1000:                      ← regular author
    for each follower_id in GetFollowerIDs:
        LPUSH following:{follower_id} <entry>
        LTRIM following:{follower_id} 0 799    ← cap at 800 entries
        EXPIRE following:{follower_id} 604800  ← 7 days

if follower_count >= 1000:                     ← celebrity
    SET celeb:{author_id} 1 EX 3600            ← skip fan-out; flag for lazy merge
```

The fan-out goroutine is bounded by a 100-slot semaphore so a burst of
high-follower tweets cannot exhaust goroutines.

On `tweet.unretweeted` the consumer scans each follower's list and `LREM`s the
matching `R|<rt_id>|...` prefix — bounded by `followingLimit` × follower count
per retweeter, run async on the consumer.

**Read path (`GET /v1/feed/following`):**
```
1. If cursor==nil and following_cache:{user_id} exists:
     return cached assembled page (skip list scan + enrichment)
   Else:
     LRANGE following:{user_id} 0 799
     ParseFeedEntry per entry → (kind, tweet_id [, retweet metadata])
     if cursor: find cursor position, slice from there
     if cursor==nil: celeb merge (first page only)
       → GetFollowingIDs → check celeb:{id} for each followee
       → GetTweetsByAuthor for celebrity followees
       → mergeDedupe(regular_entries, celeb_entries)
       → cache assembled page → following_cache:{user_id} (60s TTL)

2. Pipeline HGETALL tweet:snapshot:{id} for each unique tweet ID  ← body, embedded author, media
3. Concurrently:
     Pipeline HGETALL tweet:counts:{id} for each ID               ← counts
     GetInteractions(viewer_id, ids[])                            ← is_liked, is_retweeted
     Pipeline HGETALL user:snapshot:{retweeter_id}                ← retweeter profiles for R| entries
4. Assemble feedItem (kind, tweet, retweet?) per entry, sort by created_at DESC
5. Return items[:limit] + next_cursor
```

**Why celebrity (fan-out on read)?** For accounts with ≥1K followers, writing to each follower's list at tweet time is too expensive. Instead, we flag the author and lazily merge their tweets at read time from the gRPC call. The 1K threshold is a tuning parameter, not a hard limit.

**Cold start:** On every `user.followed` event, feed-service's consumer
calls `tweet-service.GetTweetsByAuthor(followee, limit=20)` via gRPC and
`LPUSH`es those IDs into the new follower's `following:{follower}` LIST.
Celebrities are skipped (the read-path celeb merge already covers them).
This means a brand-new user who follows 50 accounts immediately sees content
from those accounts instead of staring at an empty feed. The first-page sort
by `created_at` in feed.Service.GetFollowingFeed puts the backfilled IDs in
the correct chronological position even when mixed with live fan-out entries.

### Cursor Pagination

All paginated endpoints use opaque string cursors, not numeric page offsets.

**Why cursors over offsets?**  
`OFFSET n` requires the DB to scan and discard `n` rows — it degrades as `n` grows and produces duplicate/skipped items if new rows are inserted between pages.

Cursor pagination is `WHERE id < :cursor ORDER BY id DESC LIMIT n` — O(1) index seek regardless of page depth.

**Contract:**
- `next_cursor` is `null` on the last page, a string otherwise
- Passing a stale cursor (pointing to a deleted item) returns an empty result, not an error
- Cursors are opaque to clients — never parse them

**Different ID types for different endpoints:**
- tweet lists: cursor is the last `tw_{ulid}` (ULIDs sort lexicographically = chronologically)
- notifications: cursor is the last `ntf_{ulid}`
- following feed: cursor is the last tweet ID scanned in the Redis LIST

### Trending Hashtags

tweet-service extracts hashtags from tweet body via `#\w+` regex, includes them in `TweetCreatedEvent.Hashtags`.

feed-service Kafka consumer, on each `tweet.created`:
```
minute := time.Now().Unix() / 60
key    := "trending:minute:{minute}"
for each tag in hashtags:
    ZINCRBY  key 1 tag
EXPIRE key 70m   # window + safety margin
```

`GET /v1/feed/trending` builds a 60-minute sliding window on each read:
```
keys := trending:minute:{minute - 0..59}
ZUNIONSTORE  trending:window:{minute}  keys
EXPIRE       trending:window:{minute} 5m
ZREVRANGE    trending:window:{minute} 0 N-1 WITHSCORES
```

ZUNIONSTORE is atomic and idempotent across concurrent callers, so the per-minute destination key acts as a short-lived read cache. New hashtags appear on the next read once their bucket is incremented — no daily reset, no cumulative bias from earlier-in-the-day tags. Older buckets drop out of the window automatically as their TTL expires.

### Recommended Feed

Scoring combines two personalisation signals maintained in Redis:

- `user_interests:{user_id}` — sorted set of hashtag → affinity score (incremented when user likes/retweets a tweet with that hashtag)
- `user_affinity:{user_id}` — sorted set of author_id → affinity score (incremented on interactions)

Build process (triggered when `recommended_ts:{user_id}` is absent):
1. `GetRecentPopularTweets(limit=200)` via gRPC — candidate pool from tweet-service
2. Score each candidate: `score = sum(interest[hashtag]) + affinity[author_id]`
3. Sort descending, store IDs in `recommended:{user_id}` list
4. Set `recommended_ts:{user_id}` with 15-min TTL

Read-time enrichment is identical to the following feed (snapshots + counts + interactions).

**Cache invalidation on user activity.** Every `like` / `retweet` / `follow`
deletes `recommended_ts:{userID}` inside the same pipeline that updates the
signals — `UpdateUserInterests` and `UpdateUserAffinity`. The next read sees
the missing freshness flag and rebuilds with the just-updated signals. Without
this, recent actions wouldn't influence the feed for up to 15 minutes; with
it, the 15-min TTL becomes an *upper bound*, not a floor.

### Real-time Notifications (SSE)

notification-service runs an SSE hub backed by Redis Pub/Sub so multiple pods
can each handle a subset of connected clients without sticky sessions:

```go
type Hub struct {
    mu   sync.RWMutex
    subs map[string][]chan db.Notification  // userID → slice (one per open connection)
    rdb  *redis.Client                       // Pub/Sub fan-out across pods
}
```

When a Kafka consumer creates a notification, it `PUBLISH`es to the
`notifications` Redis channel. Every pod's `RunPubSub` goroutine receives the
message and forwards it to local in-memory subscribers for that user.

**Why a slice per user?** Multiple tabs — each `GET /v1/notifications/stream` opens a new SSE connection; all should receive the same push.

**Why non-blocking local fan-out?** If a browser tab is slow to drain, blocking
here would stall delivery for all users. Slow consumers get the event dropped —
they'll see it on the next page load.

**On connect:** the service immediately sends a `count` event with the current unread count so the client can sync its badge without a separate HTTP request.

---

## Frontend Architecture

### Why `normalize.ts` exists (critical design decision)

**Problem:** Next.js Server Components cannot import modules marked `"use client"`. TanStack Query hooks are client-only. But SSR prefetch (seeding the query cache server-side) needs to produce the same shaped objects that client hooks expect. Early code seeded the cache with raw API responses, causing a shape mismatch: when the client rehydrated, it got data in a different shape than the hooks expected, causing blank renders or double-fetches.

**Solution:** `src/lib/normalize.ts` is a plain module (no `"use client"`) containing `RawTweet`, `normalizeTweet`, and `normalizeTweetList`. Both server pages and client hooks import from it.

```ts
// Server page (SSR prefetch)
const raw = await serverGet<RawTweetListResponse>("/v1/feed/following?limit=20")
queryClient.setQueryData(["feed", "following"], {
    pages: [normalizeTweetList(raw)],
    pageParams: [undefined]
})

// Client hook (same normalizer, same output shape)
const res = await api.get<RawTweetListResponse>("/v1/feed/following")
return normalizeTweetList(res.data)
```

Cache is seeded with the correct shape on first render. No hydration mismatch.

### `redirect()` in try/catch (Next.js gotcha)

`redirect()` works by throwing a `NEXT_REDIRECT` error internally. Any surrounding `catch {}` silently swallows it, so the redirect never fires. This manifests as a missing redirect with no error message.

**Pattern used throughout the codebase:**
```ts
// WRONG — redirect() throw is caught and silently dropped
try {
    const user = await serverGet<User>(`/v1/users/${userId}`)
    if (!user.username) redirect("/onboarding")  // ← never runs
} catch {}

// CORRECT — flag inside try, redirect outside
let needsOnboarding = false
try {
    const user = await serverGet<User>(`/v1/users/${userId}`)
    needsOnboarding = !user.username
} catch {}
if (needsOnboarding) redirect("/onboarding")
```

### Route Structure

```
src/app/
├── page.tsx                        root redirect → /home
├── login/page.tsx                  "Continue with Google" — builds Keycloak auth URL
├── onboarding/
│   ├── page.tsx                    Server Component; flag-before-redirect if already onboarded
│   └── OnboardingForm.tsx          React Hook Form + Zod + useUpdateProfile
├── (main)/                         Route group — all pages here require auth
│   ├── layout.tsx                  requireUserId() → check username → /onboarding if null
│   ├── home/page.tsx               SSR prefetch following feed
│   ├── explore/page.tsx            Search UI
│   ├── notifications/page.tsx      Notification list
│   ├── profile/[id]/page.tsx       SSR prefetch user + tweets
│   └── tweet/[id]/page.tsx         SSR prefetch tweet + replies
└── api/
    ├── auth/callback/route.ts      Code exchange → set httpOnly cookies
    ├── auth/logout/route.ts        Clear cookies + Keycloak end-session
    └── notifications/stream/       Proxy SSE from notification-service
```

`(main)/layout.tsx` is the authentication gate. It calls `requireUserId()` (reads `access_token` cookie, decodes sub, redirects to `/login` if absent), then checks `username == null` and redirects to `/onboarding` if true.

### Key Libraries (`src/lib/`)

| File | Role |
|------|------|
| `normalize.ts` | Shared raw→typed normalizers; no `"use client"`; importable server + client |
| `auth.ts` | `requireUserId()` — server-only; reads cookie, decodes sub, redirects on failure |
| `types.ts` | Canonical frontend types: `Tweet`, `User`, `Notification`, `FeedResponse`, `PresignResult` |
| `schemas.ts` | Zod schemas for forms: `tweetSchema`, `replySchema`, `profileSchema`, `usernameSchema` |
| `utils.ts` | `formatCount` (empty for 0 — tweet buttons), `formatStatCount` (shows 0 — profile stats), `formatTime` |
| `server-api.ts` | `serverGet<T>` — server-side fetch with cookie auth; used only in SSR prefetch |
| `axios.ts` | Axios instance with Kong base URL and cookie-based auth |

**`formatCount` vs `formatStatCount`:** Two different zero behaviours for the same number. Tweet action buttons show nothing when count is 0 (less noisy UI). Profile follower/following counts must show `0` — users expect to see it. Using the wrong one is a silent bug.

### Optimistic Updates

Like, retweet, follow, and unfollow all use TanStack Query's `onMutate` / `onError` pattern:

1. `onMutate` — cancel in-flight queries, snapshot current cache, apply optimistic update
2. `onError` — restore snapshot
3. `onSettled` — invalidate to sync with server truth

The optimistic update patches both the individual tweet query (`["tweet", id]`) and both feed queries (`["feed", "following"]` and `["feed", "recommended"]`) simultaneously so all visible instances of the tweet update together.

---

## Infrastructure

### Local Development

`docker compose up` starts the full stack. The root `.env` file is the single
source of truth — every compose service reads its config from there (Postgres
credentials, Keycloak admin, `SERVICE_TOKEN`, `GOOGLE_CLIENT_ID`, `OPENAI_API_KEY`,
AWS dummy creds for LocalStack, etc.). Copy `.env.example` to `.env` before
first run.

`local/keycloak/realm-twitter.json` is gitignored and is rendered from
`realm-twitter.template.json` by `make generate-realm`, substituting
`${GOOGLE_CLIENT_ID}` / `${GOOGLE_CLIENT_SECRET}` from `.env`. Compose mounts
only the rendered file (not the directory) so the template's literal
placeholders never reach Keycloak's importer.

| Service | Port |
|---------|------|
| Kong proxy | 8000 |
| Kong admin | 8100 |
| user-service | 8001 (HTTP) · 9001 (gRPC) |
| tweet-service | 8002 (HTTP) · 9002 (gRPC) |
| feed-service | 8003 |
| notification-service | 8004 |
| media-service | 8005 |
| search-service | 8006 |
| Keycloak | 8080 |
| Redpanda Console | 8089 |
| PostgreSQL | 5432 |
| Redis | 6379 |
| Redpanda (Kafka) | 9092 |
| OpenSearch | 9200 |
| LocalStack (S3) | 4566 |
| Jaeger UI | 16686 |
| Prometheus | 9090 |

Every Go service exports `/metrics` (Prometheus) and emits OTLP traces over
gRPC to `jaeger:4317`. Jaeger and Prometheus run as compose services for
parity with the production EKS observability stack.

### AWS `stage` env (~$10.50/day always-on, ~$0 destroyed)

Single environment, ephemeral. `make stage-up` to bring up; `make stage-down`
between sessions. Operational runbook in [DEPLOY.md](DEPLOY.md). Terraform
code in [`infra/terraform/`](../infra/terraform/).

| Component | Spec | $/day |
|-----------|------|------:|
| EKS control plane | — | $2.40 |
| EC2 nodes | t3.medium × 2 (single node group, shared by all services) | $2.00 |
| Aurora Serverless v2 | 0.5–1 ACU, auto-pause, schema-per-service on one cluster | ~$0.50 |
| ElastiCache | cache.t4g.micro standalone | $0.41 |
| MSK | kafka.t3.small × 2 brokers (provisioned, AWS minimum) | $2.20 |
| OpenSearch | t3.small.search × 1 | $0.86 |
| NAT Gateway | 1 (2 AZs share it) | $1.08 |
| ALB | 1 (shared by web / api / auth via host rules) | $0.54 |
| Secrets Manager | ~10 secrets | $0.13 |
| CloudWatch logs | Container Insights + Fluent Bit | ~$0.50 |
| Domain + Route53 | `.xyz` registration + hosted zone | ~$0.02 |

All secrets in AWS Secrets Manager, synced to K8s Secrets via External Secrets
Operator at pod startup. IRSA per service for cloud API access (media-service →
S3, cloudwatch-agent → CloudWatch, etc.).

### Future scale-out path

Deferred for cost — design considered, code skipped. Pull in if traffic grows:

- **3-AZ HA + 3 NATs** — single-AZ outage currently takes it down. ~$3.60/day extra.
- **ElastiCache cluster mode** — needs Redis client refactor to `NewUniversalClient`.
- **MSK Serverless or multi-broker (3+ AZs)** — provisioned baseline is $18+/day, hence the 2-broker minimum here.
- **Dedicated node groups** for Kong and Keycloak — blast-radius isolation, separate scaling profiles.
- **Separate RDS for Keycloak** — currently a schema on the shared Aurora cluster.
- **WAF managed rules** — ~$15/mo, deferred as theater until there's real traffic.
- **CloudFront in front of web** — S3 presigned URLs already CDN-cache; another CDN layer is overkill until origin egress matters.
- **X-Ray distributed tracing** — the OTel SDK is already wired; would only need a CW EMF + X-Ray exporter swap.

---

## Observability & Resilience

### Current state

**Logging:** `slog` JSON to stdout on every service. Every log line includes `service` and `request_id`. Errors logged at `ERROR` level with full context before the 500 is returned to the client.

**Tracing:** OpenTelemetry. Each service emits OTLP gRPC traces to `jaeger:4317` locally. W3C `traceparent` is propagated through HTTP headers (`reqlog.TraceContext`), gRPC metadata (`otelgrpc` interceptors), and Kafka message headers (`tracing.FromKafkaMessage` / outbox enqueue stamps `traceparent`). The Jaeger UI is on `:16686`.

**Metrics:** Prometheus. Every service exports `/metrics`. The shared
`metrics` package provides Gin and gRPC middleware emitting RED metrics (RPS,
5xx, latency histogram) plus `KafkaConsumerErrors` per (service, topic) and
`OutboxDepth` per service. Local Prometheus on `:9090` scrapes the in-cluster
endpoints.

**Resilience already implemented:**
- **Idempotency:** Kafka consumers tolerate replays — feed-service / search-service use Redis `feed:dedup:{event_id}` (24h TTL); notification-service uses a `processed_events` PK table.
- **At-least-once delivery:** Transactional Outbox in tweet-service and user-service. `FOR UPDATE SKIP LOCKED` lets multiple pods flush concurrently without claiming the same rows.
- **Poison-message DLQs:** each consumer routes deserialization failures to a dedicated `<group>.dlq` topic with provenance headers; `cmd/dlq-replay` replays.
- **Circuit breakers (gobreaker):** wrap every gRPC client (`feed-service`'s `tweetclient` / `userclient`; `search-service`'s `tweetclient` / `userclient`). Open after sustained failures, half-open after a cooldown.
- **Graceful degradation:** Redis miss → empty author fields / zero counts; gRPC failure → `is_liked` / `is_retweeted` default to false; OpenAI down → keyword fallback with `X-Search-Mode: keyword-fallback`.
- **SSE hub:** non-blocking local fan-out — slow browser tabs don't stall the Kafka consumer.
- **Bounded fan-out concurrency:** 100-slot semaphore in feed-service caps inflight fan-out goroutines on a burst of high-follower tweets.

### Production observability on EKS

- **CloudWatch Container Insights** via the managed `amazon-cloudwatch-observability` EKS addon. Pod CPU / memory / network, node-level metrics, container logs to CW Logs. Replaces the older standalone CloudWatch-agent + Fluent Bit Helm dance.
- **3 CloudWatch alarms** fanned to one SNS topic (email subscriber):
  - **ALB 5xx > 10/min** — native `AWS/ApplicationELB.HTTPCode_Target_5XX_Count`.
  - **Kafka consumer lag** — `AWS/Kafka.MaxOffsetLag > 1000` sustained 3 min, per-broker.
  - **Aurora CPU > 80%** — `AWS/RDS.CPUUtilization` sustained 5 min, on the cluster identifier.

  All three use native CloudWatch metrics, so no app instrumentation. App-level alarms (outbox depth, per-service RED) require Prometheus scraping via the CW Agent Operator — deferred to the scale-out path.
- **HPA on tweet-service + feed-service** — CPU 60%, 1→5 replicas, 60s scale-up window.
- **k6 load test** — in-cluster Job, 50 feed reads + 20 posts/sec × 2min through Kong. Drives the HPA. See [`infra/k8s/loadtest/`](../infra/k8s/loadtest/).

### Resilience extras

- **`/healthz` dependency probes:** concurrent ping of each service's real deps (Postgres, Redis, OpenSearch) with a 1.5s timeout; returns 503 with a per-dep status body when any fails. ALB / k8s `readinessProbe` use this. `/livez` stays cheap and is used for liveness so transient blips don't restart pods.
- **gRPC retry with backoff + jitter** on the four cross-service clients (feed → user, feed → tweet, search → user, search → tweet). Sits inside the gobreaker boundary so the breaker observes the final outcome. 3 attempts, 50ms base, 500ms cap, full jitter; retries only on `Unavailable` / `DeadlineExceeded`, never on application-level codes. Context-cancellation aware.
- **Chaos exercises** documented at [docs/CHAOS.md](CHAOS.md) — kill feed-service / Redis, verify graceful degradation paths still serve traffic with empty author fields / zero counts.

---

## ID Conventions

| Entity | Format | Example |
|--------|--------|---------|
| User | Keycloak UUID | `550e8400-e29b-41d4-a716-446655440000` |
| Tweet | `tw_` + ULID | `tw_01HXYZ...` |
| Retweet | `rt_` + ULID | `rt_01HXYZ...` |
| Notification | `ntf_` + ULID | `ntf_01HXYZ...` |
| Media | `med_` + ULID | `med_01HXYZ...` |

**Why ULID for tweets/notifications?** ULIDs are lexicographically sortable by time — cursor pagination over tweet lists works without a `created_at` index scan. The `tw_` prefix makes IDs recognisable in logs without needing a type field.

**Why Keycloak UUID for users?** The Keycloak `sub` is the JWT's stable identifier. Using it directly as `users.id` eliminates a lookup table (`keycloak_id → local_id`) and means the `X-User-ID` header from Kong is immediately usable as a DB primary key.

