SERVICES := user-service tweet-service feed-service notification-service media-service search-service

# Every image we push to ECR. Each one has a Dockerfile at apps/<name>/Dockerfile.
# Same build pattern across all: docker build --platform linux/amd64 with repo root as context.
ECR_IMAGES := $(SERVICES) web keycloak

# Vars that must be substituted into the Keycloak realm template.
# Listed explicitly so envsubst leaves other ${...} sequences alone if any appear.
REALM_VARS := '$${GOOGLE_CLIENT_ID} $${GOOGLE_CLIENT_SECRET} $${KEYCLOAK_ADMIN_CLIENT_SECRET}'

.PHONY: dev dev-infra dev-stop env-check \
        run-user-service run-tweet-service run-feed-service run-notification-service run-media-service run-search-service run-web \
        build test test-integration test-e2e test-all generate-sqlc generate-proto generate-realm lint healthcheck help \
        stage-bootstrap stage-init stage-up stage-up-auto stage-plan stage-down stage-unlock stage-kubeconfig stage-loadtest ecr-push ecr-push-only _ecr-push-batch githooks

TF_BOOTSTRAP := infra/terraform/bootstrap
TF_STAGE     := infra/terraform/envs/stage

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

## One-shot per AWS account: state bucket + DDB lock + GitHub OIDC role + monthly budget alarms
stage-bootstrap:
	cd $(TF_BOOTSTRAP) && terraform init && terraform apply

## Initialize stage backend from bootstrap outputs. Run after stage-bootstrap or whenever .terraform/ is gone.
stage-init:
	cd $(TF_STAGE) && terraform init -reconfigure \
	  -backend-config="bucket=$$(terraform -chdir=../../bootstrap output -raw state_bucket)" \
	  -backend-config="region=$$(terraform -chdir=../../bootstrap output -raw region)"

## Provision the stage environment (Phase 9 chunks 1-10).
## Runs as TWO phases because the alekc/kubectl provider can't configure when
## the EKS cluster endpoint is unknown (cluster doesn't exist yet on first apply):
##   Phase 1: full VPC + EKS cluster + helm addons (you confirm 'yes')
##   Phase 2: data plane + ECR + ExternalSecrets (you confirm 'yes' again)
## After everything is up, both phases become no-ops for unchanged resources.
## NOTE: -target=module.vpc is essential. -target=module.eks_cluster alone only
## creates VPC + subnets (the direct refs) and skips NAT/route tables/IGW, leaving
## nodes with no internet egress and unable to reach the EC2 API to bootstrap.
stage-up: stage-init
	cd $(TF_STAGE) && terraform apply -target=module.vpc -target=module.eks_cluster -target=module.eks_addons
	$(MAKE) stage-kubeconfig
	cd $(TF_STAGE) && terraform apply

## Like stage-up but auto-confirms both phases (unattended / CI runs / Phase 10).
stage-up-auto: stage-init
	cd $(TF_STAGE) && terraform apply -target=module.vpc -target=module.eks_cluster -target=module.eks_addons -auto-approve
	$(MAKE) stage-kubeconfig
	cd $(TF_STAGE) && terraform apply -auto-approve

## Plan changes against the stage environment.
stage-plan: stage-init
	cd $(TF_STAGE) && terraform plan

## Tear down the stage environment. Bootstrap stack survives so state is preserved for the next stage-up.
stage-down:
	cd $(TF_STAGE) && terraform destroy

## Build + push all 8 ECR images (6 Go services + web + keycloak). Tags with git short SHA + `latest`.
## --platform linux/amd64 is required when building on an arm64 Mac (EKS nodes are amd64).
ecr-push:
	@IMAGES="$(ECR_IMAGES)" $(MAKE) -s _ecr-push-batch

## Push a subset. Example: make ecr-push-only IMAGES="keycloak web"
ecr-push-only:
	@test -n "$(IMAGES)" || { echo 'Usage: make ecr-push-only IMAGES="keycloak web"'; exit 1; }
	@$(MAKE) -s _ecr-push-batch

# Internal: shared push loop. Caller passes IMAGES.
_ecr-push-batch:
	@ACCOUNT_ID=$$(aws sts get-caller-identity --query Account --output text) && \
	  REGION=$$(terraform -chdir=$(TF_BOOTSTRAP) output -raw region) && \
	  REGISTRY="$$ACCOUNT_ID.dkr.ecr.$$REGION.amazonaws.com" && \
	  TAG=$$(git rev-parse --short HEAD) && \
	  aws ecr get-login-password --region "$$REGION" | docker login --username AWS --password-stdin "$$REGISTRY" && \
	  for img in $(IMAGES); do \
	    echo "==> $$img"; \
	    docker build --platform linux/amd64 \
	      -t "$$REGISTRY/$$img:$$TAG" -t "$$REGISTRY/$$img:latest" \
	      -f apps/$$img/Dockerfile . || exit 1; \
	    docker push "$$REGISTRY/$$img:$$TAG" || exit 1; \
	    docker push "$$REGISTRY/$$img:latest" || exit 1; \
	  done
	@echo ""
	@echo "Pushed tag = $$(git rev-parse --short HEAD) (also tagged latest)"

## Write a local kubeconfig pointing at the stage EKS cluster. Run after stage-up.
stage-kubeconfig:
	@REGION=$$(terraform -chdir=$(TF_BOOTSTRAP) output -raw region) && \
	  NAME=$$(terraform -chdir=$(TF_STAGE) output -raw eks_cluster_name) && \
	  aws eks update-kubeconfig --region "$$REGION" --name "$$NAME" --alias twitter-mc-stage

## Remove a stale S3 state lock. Use when an interrupted apply/destroy left an orphan tflock.
## Verify nothing is actually running first: `ps aux | grep terraform | grep -v grep` should be empty.
stage-unlock:
	@BUCKET=$$(terraform -chdir=$(TF_BOOTSTRAP) output -raw state_bucket) && \
	  REGION=$$(terraform -chdir=$(TF_BOOTSTRAP) output -raw region) && \
	  aws s3 rm "s3://$$BUCKET/stage/terraform.tfstate.tflock" --region "$$REGION"

## Run the in-cluster k6 load test (50 reads + 20 posts/sec x 2min via Kong).
## Pre-req: BASE_URL + JWT_TOKEN already set as k6-env ConfigMap/Secret in the apps namespace.
## See infra/k8s/loadtest/README.md for the token-fetch + env-setup recipe.
stage-loadtest:
	@kubectl --context twitter-mc-stage -n apps delete job k6-loadtest --ignore-not-found
	@kubectl --context twitter-mc-stage apply -f infra/k8s/loadtest/
	@echo "Tail logs:  kubectl --context twitter-mc-stage -n apps logs -f job/k6-loadtest"
	@echo "Watch HPA:  kubectl --context twitter-mc-stage -n apps get hpa -w"

## Activate the repo's git hooks (terraform fmt on commit). Run once per clone.
githooks:
	@git config core.hooksPath .githooks && echo "git hooks activated from .githooks/"

## Show available targets
help:
	@grep -E '^##' Makefile | sed 's/## //'
