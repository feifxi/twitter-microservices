# k6 Load Test

In-cluster Job that drives Kong with 50 feed reads/sec + 20 tweet posts/sec
for 2 minutes — enough to push `tweet-service` and `feed-service` past the
HPA's 60% CPU target so you can watch `kubectl get hpa -w` scale 1 → N.

## Run

```bash
# 1. Get a JWT for a test user. Direct-grant flow against Keycloak:
TOKEN=$(curl -s -X POST \
  https://auth.twitter.<your-domain>/realms/twitter/protocol/openid-connect/token \
  -d grant_type=password \
  -d client_id=twitter-app \
  -d username=loadtest \
  -d password=loadtest \
  | jq -r .access_token)

# 2. Apply env (BASE_URL ConfigMap + JWT_TOKEN Secret):
kubectl -n apps create configmap k6-env \
  --from-literal=BASE_URL="https://api.twitter.<your-domain>" \
  --dry-run=client -o yaml | kubectl apply -f -

kubectl -n apps create secret generic k6-env \
  --from-literal=JWT_TOKEN="$TOKEN" \
  --dry-run=client -o yaml | kubectl apply -f -

# 3. Apply script ConfigMap + Job:
kubectl apply -f infra/k8s/loadtest/

# 4. Tail logs + watch HPA in parallel:
kubectl -n apps logs -f job/k6-loadtest &
kubectl -n apps get hpa -w
```

## Cleanup

The Job has `ttlSecondsAfterFinished: 3600` so it self-deletes after an
hour. To re-run sooner:

```bash
kubectl -n apps delete job k6-loadtest
kubectl apply -f infra/k8s/loadtest/job.yaml
```
