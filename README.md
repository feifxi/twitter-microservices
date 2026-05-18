# Twitter Microservices

Cloud-native microservices Twitter clone — learning project for distributed systems, event-driven architecture, and SRE practice.

![Preview](docs/images/preview.png)

## Stack

| Component | Local Dev | Production (AWS) |
|-----------|-----------|-----------------|
| Language / framework | Go 1.26 · Gin · sqlc | *(same)* |
| Frontend | Next.js 16 | *(same)* |
| API Gateway | Kong | *(same)* |
| Auth | Keycloak | *(same)* |
| Database | PostgreSQL | Aurora PostgreSQL Serverless v2 |
| Cache | Redis | ElastiCache |
| Message broker | Redpanda | Amazon MSK (Kafka) |
| Search | OpenSearch | Amazon OpenSearch Service |
| Object storage | LocalStack (S3) | Amazon S3 |
| Orchestration | Docker Compose | Amazon EKS · Terraform |
| Observability | slog · Prometheus · OpenTelemetry → Jaeger | slog · CloudWatch · OpenTelemetry → X-Ray |

## Services

| Service | Local HTTP | gRPC | Responsibility |
|---|---|---|---|
| user-service | 8001 | 9001 | Profile, follow graph, Keycloak provisioning |
| tweet-service | 8002 | 9002 | Tweets, likes, retweets, replies, `user:snapshot` writer |
| feed-service | 8003 | — | Fan-out feed, trending |
| notification-service | 8004 | — | Notifications |
| media-service | 8005 | — | S3 presigned uploads |
| search-service | 8006 | — | OpenSearch tweet + user search |
| web (Next.js) | 3000 | — | Frontend |

All traffic in prod goes through **Kong on :8000**. Internal ports above are local-dev only.

## Prerequisites

Docker Desktop · Go 1.26+ · Node.js 22+ · Make · `jq`.

## Quick Start

```bash
cp .env.example .env          # fill in OPENAI_API_KEY and Google OAuth creds
make dev                      # build + run everything
make healthcheck              # verify all services healthy
make run-web                  # start Next.js (optional)
```

Open [http://localhost:3000/login](http://localhost:3000/login).

`make dev-stop` tears everything down.

## Make Targets

```
make dev               # build + run full stack
make dev-infra         # only backing stores (Postgres, Redis, Kafka, S3, OpenSearch, Keycloak, Kong)
make run-<service>     # run one Go service against dev-infra
make run-web           # Next.js dev server
make test              # unit tests
make test-integration  # integration tests (needs Docker)
make generate-sqlc     # regen sqlc after editing db/query.sql
make generate-proto    # regen protobuf after editing .proto
make generate-realm    # render Keycloak realm from .env (auto-runs before dev)
make lint              # go vet all services + biome on frontend
make healthcheck       # curl all /healthz endpoints
```

## Local URLs

| Resource | URL |
|---|---|
| Kong gateway | http://localhost:8000 |
| Keycloak (admin / admin) | http://localhost:8080 |
| Redpanda Console | http://localhost:8089 |
| Jaeger | http://localhost:16686 |
| Prometheus | http://localhost:9090 |
| OpenSearch | http://localhost:9200 |
| LocalStack S3 | http://localhost:4566 |

## Auth

Keycloak (OIDC + Google IdP) → Kong (JWT validation) → injects `X-User-ID` / `X-User-Email` / `X-User-Username` headers. Services never handle tokens.

## Observability

Every service emits JSON logs (`slog`), Prometheus metrics on `/metrics`, and OTLP traces. W3C `traceparent` is propagated through HTTP, gRPC, and Kafka headers so a single request can be followed from Kong → service → outbox → consumer → SSE in one Jaeger trace.

## Docs

- [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) — service layout, data models, Kafka contracts, Redis schema, conventions

## Troubleshooting

- **`make dev` fails: docker daemon not running** — start Docker Desktop.
- **Port in use** — `make dev-stop` first.
- **Healthcheck fails after start** — wait 30s for backing stores; Keycloak is the slowest (~30s).
- **Keycloak "Could not send authentication request"** — your `.env` has empty Google creds. Fill in `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET`, then `make generate-realm && docker compose restart keycloak`.
- **Search returns 502** — `OPENAI_API_KEY` unset or invalid. Search-service `MustEnv`s on startup.
