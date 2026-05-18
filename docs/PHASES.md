# Build Phases

History of how the project was assembled. For architecture, API shapes, and
data models see [ARCHITECTURE.md](ARCHITECTURE.md). For current SRE-style
status (resilience, observability) see the **Observability & Resilience**
section in that doc.

## Process

Each phase ends with: code cross-check against checklist items, `make test` +
`make build` clean, docs in sync.

---

## Phase 0 — Monorepo Scaffold ✅
Directory layout, Go service bootstraps, Next.js App Router, docker-compose, Makefile.

---

## Phase 1 — User Service + Auth ✅
`users` + `follows` schema, Keycloak Required Action SPI, `POST /internal/provision` (idempotent), profile CRUD on `/v1/users/me`, follow/unfollow, follower/following lists, suggestions, gRPC server (`GetFollowerIDs`, `GetFollowerCount`, `GetFollowingIDs`, `BatchGetFollowerCounts`, `GetFollowState`, `GetUserByID`), Transactional Outbox for `user.*` events, Keycloak admin sync for `preferred_username`, integration tests.

---

## Phase 2 — Tweet Service ✅
`tweets`, `retweets`, `likes`, `outbox` schema. Full tweet CRUD, like/unlike (idempotent), reference-only retweet with `UNIQUE (retweeter_id, original_tweet_id)`, reply with CTE, hashtag extraction, gRPC server, Outbox goroutine, Kafka events, `RunUserSnapshotConsumer` (sole writer of `user:snapshot`), `tweet:counts` HINCRBY on mutations, profile timeline tabs (`posts` / `replies` / `media` / `likes`), integration tests.

---

## Phase 3 — Feed Service ✅
Kafka consumers for 7 topics. Fan-out with celebrity threshold (≥1K), bounded by a 100-slot semaphore. Redis denormalisation: `tweet:snapshot` (embedded author), `author_tweets`, `following:*` (T|/R| feed entries), affinity signals. Following feed (cursor + celeb merge + GetInteractions + 60s assembled-page cache). Recommended feed (interest + affinity scoring, 15-min TTL cache). Trending (per-minute ZSETs + ZUNIONSTORE 60-min sliding window, auto-decay via TTL). Integration tests.

---

## Phase 4 — Media Service ✅
`POST /v1/media/presign`, LocalStack S3 in docker-compose. tweet-service accepts `media_id`/`media_url`. Integration test: presign → PUT → tweet with media → verify.

---

## Phase 5 — Notification Service ✅
`notifications` + `processed_events` schema, `notif_type` enum. Kafka consumers for like/retweet/unretweet/reply/follow events. Paginated history with Redis-snapshot enrichment. Mark-all/mark-one read. SSE hub backed by Redis Pub/Sub (multi-pod, multi-tab, non-blocking). Self-notification skipped (actor == recipient). Per-event dedup via `processed_events` table with periodic prune. Integration tests.

---

## Phase 6a — Embedding Service (Decision: Dropped) ✅
Originally planned as a separate Python service. Replaced by direct OpenAI API calls from search-service (`internal/embeddings/client.go`). Saves a container and CGO complexity; provider swappable via env vars. Fallback to keyword search if API unavailable.

---

## Phase 6b — Search Service ✅
Go service with OpenSearch 3.5. `tweets` + `users` indices created at startup. Kafka consumer `search-service-cg` for index sync. Keyword (BM25), semantic (kNN 256-dim), hybrid modes. `X-Search-Mode: keyword-fallback` header on fallback. Read-time enrichment: pipelined `user:snapshot` + `tweet:counts` from Redis plus `tweet-service.GetInteractions` and `user-service.BatchGetFollowerCounts` / `GetFollowState` over gRPC (all gobreaker-wrapped). Integration tests.

---

## Phase 7 — Frontend Auth + Onboarding ✅
OAuth 2.0 Authorization Code flow, httpOnly cookie storage, silent token refresh middleware (`src/middleware.ts`), login page, auth callback, logout (Keycloak end-session), app shell with TanStack Query + Zustand providers.

---

## Phase 8 — Frontend (Next.js 16.2) ✅
- SSR-prefetched timeline/tweet/profile pages (`normalize.ts` pattern)
- Optimistic like/retweet/follow/unfollow
- Tweet compose with media upload
- Reply thread view
- Recommended feed tab
- Trending sidebar (30s poll)
- Tweet + user search
- Notification bell + SSE live push + badge count
- Profile page + edit modal
- Onboarding with redirect guard (flag-before-redirect pattern)
- Dark/light theme (`next-themes`)
- Shared `Avatar`, `TweetSkeleton`, `BackButton` components
- `formatCount` vs `formatStatCount` distinction
- Biome lint/format
- Playwright E2E (`apps/web/e2e/`) — post → profile timeline → trending → search. Direct-grant against Keycloak with cookie-based storageState (sidesteps the interactive Google OAuth dance); idempotent test-user provisioning via Keycloak admin API + user-service `/internal/provision`. `make test-e2e`.

---

## Phase 9 — AWS Infrastructure (Terraform + EKS) 🔄 In Progress
VPC (3 AZs, 1 NAT), EKS cluster, ECR repos, Aurora Serverless v2, ElastiCache, MSK, OpenSearch Service, Secrets Manager + External Secrets Operator, Keycloak production Helm chart (`start` mode, TLS, JWKS-based Kong JWT), ALB + WAF, CloudFront, smoke tests.

---

## Phase 10 — CI/CD Pipeline ⬜
GitHub Actions: `ci.yml` (vet + test + build matrix per service + Biome), `deploy.yml` (integration → ECR → helm upgrade), `infra.yml` (terraform plan on PR / apply on merge). OIDC auth to AWS (no stored keys). Branch protection: CI must pass, linear history.

---

## Phase 11 — Observability ✅ (local) / 🔄 (production)
**Shipped locally:** OTel exporter to Jaeger over OTLP gRPC, W3C `traceparent`
propagated through HTTP / gRPC / Kafka headers; auto-instrumented Gin (via
shared `reqlog.TraceContext` + `metrics.GinMiddleware`), gRPC (`otelgrpc`),
Redis, pgx. Prometheus `/metrics` endpoint on every service, scraped by a local
Prometheus instance.

**Remaining for production:** CloudWatch EMF mapping for the existing Prometheus
surface, Container Insights, alarms (5xx > 10/min → SNS, Kafka lag > 30s,
outbox depth growing), `version` field in logs via `-ldflags`, X-Ray exporter
in prod.

---

## Phase 12 — Resilience & Load Testing 🔄 In Progress
**Shipped:** Circuit breakers (gobreaker) wrap every gRPC client in
feed-service and search-service; graceful degradation across the stack
(Redis miss → empty fields / fallback to OpenSearch denormalised counts;
gRPC failure → `is_liked`/`is_retweeted=false`; OpenAI down → keyword
fallback); DLQs per consumer group with provenance headers; bounded fan-out
concurrency.

**Shipped this phase:** `/healthz` dep probes + `/livez` split; gRPC retry interceptor (backoff + jitter, inside the breaker boundary).
**Remaining local:** chaos exercises (kill feed-service, kill Redis) documented as a procedure.
**Deferred to Phase 9:** HPA (CPU 60%) and k6 load test — neither produces a meaningful signal against docker-compose.

---

## Appendix — Airflow Discussion (not scheduled)

One future job that would benefit from Airflow visibility once the system matures:

1. **Nightly analytics export** — not yet implemented; would export Postgres aggregates → S3 CSV for reporting.

Airflow adds: UI visibility, automatic retry, backfill for missed runs, built-in alerting — vs a "goroutine that logs a warning on failure."
