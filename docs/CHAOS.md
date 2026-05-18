# Chaos Procedures (Local)

Hand-driven validation of the graceful-degradation paths described in
ARCHITECTURE.md → *Observability & Resilience*. Each experiment kills one
dependency and confirms the system stays serving with a documented fallback
rather than 5xx-ing everything.

This is **not** chaos engineering in the Chaos Monkey / Litmus sense — there's
no automation, no fault injection framework, no SLO-driven scoring. It's a
local runbook a human runs by hand against `docker compose`. Automated chaos
belongs against EKS in Phase 9.

## Prerequisites

- Stack up: `make dev`
- Frontend up: `make run-web`
- A logged-in browser session at http://localhost:3000 with at least one tweet
  visible in the feed (so empty results are unambiguous).
- Optional: open Jaeger ([http://localhost:16686](http://localhost:16686)) in a
  second tab so traces are visible while you break things.

Recovery is always `docker compose start <service>` and verifying
`curl -sf http://localhost:8000` followed by `make healthcheck`.

---

## 1. Redis down — read-path enrichment falls back

**Hypothesis.** With Redis unreachable, the feed, profile, and search pages
still render. Author fields appear empty, counts appear zero, but pages do not
500. This validates the `tweet:snapshot` / `user:snapshot` / `tweet:counts`
miss paths plus search-service's fallback to OpenSearch denormalised counts.

**Break.** `docker compose stop redis`

**Verify.**

- `curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8003/healthz`
  returns **503** with `deps[].name="redis", status="error"`.
- `curl -s -o /dev/null -w "%{http_code}\n" http://localhost:8003/livez`
  returns **200** (process is healthy, dep is not — no pod restart).
- Reload the home feed in the browser. The recommended/following lists may
  empty (Redis is the read path for both), but the **profile page**
  (`/profile/{me}`) still renders the timeline because tweet-service reads
  Postgres directly. Author names render as the bare ID; counts show 0.
- Search still returns hits (OpenSearch is untouched), but author fields and
  counts fall back to the values denormalised into the OpenSearch doc at
  index time.

**Recover.** `docker compose start redis`. Snapshots and counters rebuild
lazily as new events flow through.

---

## 2. feed-service down — frontend error path

**Hypothesis.** `/v1/feed/following` and `/v1/feed/recommended` 502 through
Kong. There is no alternative source for the home feed, so the test here is
that the **frontend** handles the error cleanly: spinner stops, error UI
renders, no infinite reload loop.

**Break.** `docker compose stop feed-service`

**Verify.**

- `curl -i http://localhost:8000/v1/feed/trending` returns **502** within ~1s
  (Kong upstream timeout, not a 30s hang).
- Home page (`/home`) shows the empty/error state, not a frozen skeleton.
- Posting a new tweet still succeeds — tweet-service is unaffected, and the
  outbox row queues for the eventual feed-service restart.
- Notification SSE keeps streaming — notification-service is independent.

**Recover.** `docker compose start feed-service`. On startup, the consumer
resumes from the committed Kafka offset; tweets posted during the outage
fan out on the catch-up pass. The 60s assembled-page cache in feed-service
will be empty, so first reloads are slightly slower while it rebuilds.

---

## 3. user-service gRPC down — breaker opens, viewer flags degrade

**Hypothesis.** feed-service and search-service make gRPC calls to user-service
for follower counts and follow-state. Both clients are wrapped in gobreaker
and the new retry interceptor (`apps/shared/grpcretry`). With user-service
down: retries fire 3× with backoff+jitter, the breaker observes failures and
opens, subsequent reads short-circuit to the documented fallback
(`is_liked` / `is_retweeted` default to false; counts come from Redis).

**Break.** `docker compose stop user-service`

**Verify.**

- `docker compose logs feed-service --since 30s | grep -i breaker` shows the
  breaker transitioning to `Open`. Until it opens, individual calls show 3
  attempts spaced ~50–500ms apart (the retry interceptor's backoff).
- `/v1/feed/recommended` keeps returning 200. Items render with author from
  Redis snapshots, but `is_liked`/`is_retweeted` are uniformly false.
- Trying to fetch `/v1/users/:id` directly returns 502 (user-service is the
  only source).

**Recover.** `docker compose start user-service`. The breaker stays open
until the cooldown window (gobreaker default ~60s) elapses, then probes
half-open with the next request. After that, real responses flow again and
`is_liked`/`is_retweeted` repopulate on the next refetch.

---

## 4. OpenAI down — search falls back to keyword

**Hypothesis.** Semantic search calls OpenAI for the query embedding. If the
embedding call fails, search-service falls back to BM25-only and signals it
via the `X-Search-Mode: keyword-fallback` response header.

**Break.** Override the key with garbage and restart search-service:

```bash
docker compose exec -T search-service sh -c 'OPENAI_API_KEY=invalid /service' &
# or simpler — edit .env, set OPENAI_API_KEY=invalid, then:
docker compose up -d --force-recreate search-service
```

**Verify.**

- `curl -i 'http://localhost:8000/v1/search/tweets?q=hello&mode=hybrid' \
   -H 'Authorization: Bearer $TOKEN'` returns 200 with
  `X-Search-Mode: keyword-fallback` in the response headers.
- Results are still relevant — BM25 keyword scoring takes over.
- `search-service` logs show one warning per request about the embedding
  failure, not a stack trace per request (don't spam logs).

**Recover.** Restore `OPENAI_API_KEY` in `.env` and
`docker compose up -d --force-recreate search-service`. Hybrid mode resumes
on the next request.

---

## 5. Kafka consumer lag — outbox catches up

**Hypothesis.** Stop feed-service while traffic continues. Posts written to
tweet-service's `outbox` table stay durable; new `tweet.created` messages
land in Kafka but are not consumed. When feed-service comes back, it
catches up from the committed offset — no events are lost, trending and
fan-out eventually reflect the missed window.

**Break.**

```bash
docker compose stop feed-service
# In the browser, post 3–5 tweets with unique hashtags.
docker compose exec -T redpanda rpk group describe feed-service-cg \
  | grep tweet.created
# LAG column should be > 0.
docker compose start feed-service
```

**Verify.**

- After ~5–10s the same `rpk group describe` shows `LAG = 0`.
- The new hashtags appear in `GET /v1/feed/trending?limit=200`.
- The new tweets appear in followers' home feeds (you'll only see this
  if you actually have followers — otherwise check via profile timeline).

**Recover.** Already recovered — the experiment is the recovery.

---

## What this validates

| Path | Experiment |
|------|------------|
| `tweet:snapshot` / `user:snapshot` Redis miss | 1 |
| Frontend error UI on upstream 5xx | 2 |
| gobreaker open + retry interceptor backoff | 3 |
| Viewer-flag fallback (`is_liked`=false) | 3 |
| OpenAI degraded → keyword fallback header | 4 |
| Kafka durability + consumer catch-up | 5 |
| `/healthz` 503 vs `/livez` 200 split | 1 |

## What this does **not** validate

- Behavior under sustained load (Phase 9, k6 against EKS).
- Partition / network-split scenarios (single-host docker compose).
- Pod-level restart, OOMKill, or graceful shutdown timing (no orchestrator).
- DLQ replay and poison-message handling — exercise those separately by
  publishing a malformed event to a consumer's topic and watching the DLQ
  partition.
