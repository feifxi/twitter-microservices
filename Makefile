SERVICES := user-service tweet-service feed-service notification-service media-service search-service

# Vars that must be substituted into the Keycloak realm template.
# Listed explicitly so envsubst leaves other ${...} sequences alone if any appear.
REALM_VARS := '$${GOOGLE_CLIENT_ID} $${GOOGLE_CLIENT_SECRET} $${KEYCLOAK_ADMIN_CLIENT_SECRET}'

.PHONY: dev dev-infra dev-stop env-check \
        run-user-service run-tweet-service run-feed-service run-notification-service run-media-service run-search-service run-web \
        build test test-integration test-e2e test-all generate-sqlc generate-proto generate-realm lint healthcheck help

## Verify .env exists and required vars are set
# GOOGLE_CLIENT_ID/SECRET are intentionally NOT required — Google IdP is optional.
# If empty, Keycloak's Google login button is non-functional but email/password still works.
env-check:
	@test -f .env || { echo "ERROR: .env missing. Copy .env.example to .env and fill in values."; exit 1; }
	@set -a && . ./.env && set +a && \
	  for v in OPENAI_API_KEY POSTGRES_USER POSTGRES_PASSWORD POSTGRES_DB SERVICE_TOKEN KEYCLOAK_ADMIN_CLIENT_SECRET; do \
	    if [ -z "$$(eval echo \$$$$v)" ]; then echo "ERROR: $$v is empty in .env"; exit 1; fi; \
	  done
	@set -a && . ./.env && set +a && \
	  if [ -z "$$GOOGLE_CLIENT_ID" ]; then echo "note: GOOGLE_CLIENT_ID is empty — Google login disabled"; fi

## Start all services and backing stores (Postgres, Redis, Redpanda, LocalStack)
dev: generate-realm
	docker compose up --build

## Start only backing stores + Kong + Keycloak (useful when running services locally via `go run`)
dev-infra: generate-realm
	docker compose up postgres redis redpanda localstack kong keycloak opensearch

## Stop all containers and remove volumes
dev-stop:
	docker compose down -v

## Run user-service locally (requires: make dev-infra)
run-user-service: env-check
	@cd apps/user-service && set -a && . ../../.env && set +a && \
	  MIGRATIONS_PATH="file://migrations" \
	  DATABASE_URL="postgres://$$POSTGRES_USER:$$POSTGRES_PASSWORD@localhost:5432/$$POSTGRES_DB?sslmode=disable&search_path=users" \
	  MIGRATIONS_TABLE="user_migrations" \
	  REDIS_URL="localhost:6379" \
	  KAFKA_BROKERS="localhost:9092" \
	  KEYCLOAK_BASE_URL="http://localhost:8080" \
	  KEYCLOAK_REALM="twitter" \
	  GRPC_PORT="9090" \
	  PORT="8001" \
	  go run ./cmd/server

## Run tweet-service locally (requires: make dev-infra + user-service running)
run-tweet-service: env-check
	@cd apps/tweet-service && set -a && . ../../.env && set +a && \
	  MIGRATIONS_PATH="file://migrations" \
	  DATABASE_URL="postgres://$$POSTGRES_USER:$$POSTGRES_PASSWORD@localhost:5432/$$POSTGRES_DB?sslmode=disable&search_path=tweet" \
	  MIGRATIONS_TABLE="tweet_migrations" \
	  REDIS_URL="localhost:6379" \
	  KAFKA_BROKERS="localhost:9092" \
	  USER_SERVICE_GRPC_ADDR="localhost:9090" \
	  GRPC_PORT="9091" \
	  PORT="8002" \
	  go run ./cmd/server

## Run feed-service locally (requires: make dev-infra + user-service + tweet-service running)
run-feed-service: env-check
	@cd apps/feed-service && set -a && . ../../.env && set +a && \
	  REDIS_URL="localhost:6379" \
	  KAFKA_BROKERS="localhost:9092" \
	  USER_SERVICE_GRPC_ADDR="localhost:9090" \
	  TWEET_SERVICE_GRPC_ADDR="localhost:9091" \
	  PORT="8003" \
	  go run ./cmd/server

## Run notification-service locally (requires: make dev-infra)
run-notification-service: env-check
	@cd apps/notification-service && set -a && . ../../.env && set +a && \
	  MIGRATIONS_PATH="file://migrations" \
	  DATABASE_URL="postgres://$$POSTGRES_USER:$$POSTGRES_PASSWORD@localhost:5432/$$POSTGRES_DB?sslmode=disable&search_path=notification" \
	  MIGRATIONS_TABLE="notification_migrations" \
	  KAFKA_BROKERS="localhost:9092" \
	  PORT="8004" \
	  go run ./cmd/server

## Run search-service locally (requires: make dev-infra + opensearch running)
run-search-service: env-check
	@cd apps/search-service && set -a && . ../../.env && set +a && \
	  OPENSEARCH_URL="http://localhost:9200" \
	  OPENAI_EMBEDDING_MODEL="text-embedding-3-small" \
	  KAFKA_BROKERS="localhost:9092" \
	  PORT="8006" \
	  go run ./cmd/server

## Run media-service locally (requires: make dev-infra)
run-media-service: env-check
	@cd apps/media-service && set -a && . ../../.env && set +a && \
	  AWS_ENDPOINT_URL="http://localhost:4566" \
	  S3_BUCKET=twitter-media \
	  MEDIA_PUBLIC_URL_BASE="http://localhost:4566/twitter-media" \
	  PORT="8005" \
	  go run ./cmd/server

## Run Next.js frontend locally (requires: make dev-infra)
run-web:
	cd apps/web && npm install && npm run dev

## Build all Go service binaries (smoke check — not Docker)
build:
	@for svc in $(SERVICES); do \
	  echo "→ building $$svc"; \
	  (cd apps/$$svc && go build ./...); \
	done

## Run unit tests for all Go services
test:
	@failed=0; \
	for svc in $(SERVICES); do \
	  echo "→ testing $$svc"; \
	  (cd apps/$$svc && go test -race -count=1 ./...) || failed=1; \
	done; \
	exit $$failed

## Run integration tests (requires Docker — spins up real containers)
test-integration:
	@failed=0; \
	for svc in $(SERVICES); do \
	  echo "→ integration testing $$svc"; \
	  (cd apps/$$svc && go test -tags integration -timeout 120s -race ./...) || failed=1; \
	done; \
	exit $$failed

## Run all tests
test-all: test test-integration

## Re-run sqlc codegen after query.sql changes (uses Docker to avoid CGO issues on macOS)
generate-sqlc:
	@for svc in user-service tweet-service notification-service; do \
	  echo "→ sqlc generate $$svc"; \
	  docker run --rm -v "$(CURDIR)/apps/$$svc:/src" -w /src sqlc/sqlc:1.30.0 generate; \
	done

## Re-run protobuf codegen after .proto file changes
generate-proto:
	@echo "→ buf generate"
	@docker run --rm -v "$(CURDIR)/apps/shared/proto:/workspace" -w /workspace bufbuild/buf:latest generate

## Render local/keycloak/realm-twitter.json from the template using .env values
generate-realm: env-check
	@set -a && . ./.env && set +a && \
	  envsubst $(REALM_VARS) < local/keycloak/realm-twitter.template.json > local/keycloak/realm-twitter.json
	@echo "→ rendered local/keycloak/realm-twitter.json"

## Run linters (go vet + biome)
lint:
	@for svc in $(SERVICES); do \
	  echo "→ vet $$svc"; \
	  (cd apps/$$svc && go vet ./...); \
	done
	@echo "→ biome (web)"
	(cd apps/web && npx biome check .)

## Run Playwright E2E suite against the local stack (requires: make dev + make run-web)
test-e2e: env-check
	@# Keycloak master realm defaults to sslRequired=external, which rejects direct
	@# grant over HTTP. Idempotent; only runs once per stack lifetime in practice.
	@set -a && . ./.env && set +a && \
	  docker compose exec -T keycloak /opt/keycloak/bin/kcadm.sh config credentials \
	    --server http://localhost:8080 --realm master \
	    --user "$$KEYCLOAK_ADMIN" --password "$$KEYCLOAK_ADMIN_PASSWORD" >/dev/null && \
	  docker compose exec -T keycloak /opt/keycloak/bin/kcadm.sh update realms/master -s sslRequired=NONE >/dev/null
	@cd apps/web && \
	  set -a && . ../../.env && set +a && \
	  npx --no-install playwright install chromium >/dev/null 2>&1 || npx playwright install chromium && \
	  npm run test:e2e

## Verify all healthz endpoints are up (requires dev stack running)
healthcheck:
	@echo "Checking all services..."
	@curl -sf http://localhost:8001/healthz | jq .
	@curl -sf http://localhost:8002/healthz | jq .
	@curl -sf http://localhost:8003/healthz | jq .
	@curl -sf http://localhost:8004/healthz | jq .
	@curl -sf http://localhost:8005/healthz | jq .
	@curl -sf http://localhost:8006/healthz | jq .

## Show available targets
help:
	@grep -E '^##' Makefile | sed 's/## //'
