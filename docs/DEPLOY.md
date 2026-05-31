# Deploy Runbook — AWS Stage Environment

The infrastructure for the public stage env (`https://twitter.chanombude.me`) lives in `infra/terraform/`. This runbook walks through provisioning from cold, the manual touch points along the way, and tear-down.

## Prereqs (one-time per machine)

```bash
brew install terraform awscli kubectl jq
aws configure                # IAM user with Admin or equivalent in your AWS account
```

## One-time per AWS account: bootstrap

Already applied. If you ever wipe the bootstrap stack, recreate with:

```bash
cp infra/terraform/bootstrap/terraform.tfvars.example infra/terraform/bootstrap/terraform.tfvars
# edit budget_email if you want budget alerts elsewhere
make stage-bootstrap          # ~2 min — creates S3 state bucket + GitHub OIDC role + SNS + $50/mo budget
```

Confirm the SNS subscription email when it arrives.

## Cold start (after `make stage-down` or first deploy)

```bash
# 1. Push images to ECR (~5-10 min first time, faster on rebuilds)
make ecr-push                  # builds + pushes all 8 images (6 Go services + web + keycloak)

# 2. Provision AWS infra + deploy apps (~30 min wall-clock)
make stage-up-auto             # two-phase apply, no prompts; use `make stage-up` if you want to review the plan
```

During the apply, terraform will print the Route53 nameservers in a banner box like:

```
================================================================================
  Route53 zone for twitter.chanombude.me created.
  ADD THESE 4 NS RECORDS AT YOUR REGISTRAR FOR SUBDOMAIN 'twitter':
    ns-XXXX.awsdns-XX.org
    ns-XXXX.awsdns-XX.co.uk
    ns-XXXX.awsdns-XX.net
    ns-XXXX.awsdns-XX.com
================================================================================
```

**While terraform keeps running, add the 4 NS records at Namecheap:**

1. Sign in → Domain List → `chanombude.me` → Manage → Advanced DNS
2. Delete any existing NS records on host `twitter` (from a previous deploy)
3. Add 4 new NS records — Type: NS, Host: `twitter`, Value: one nameserver per record
4. Save

DNS propagates in ~5 min. Terraform's ACM validation step hangs until propagation succeeds, then everything proceeds. **Do not Ctrl+C** while it waits.

When the apply finishes, you get final outputs:

```
web_url      = "https://twitter.chanombude.me"
api_url      = "https://api.twitter.chanombude.me"
auth_url     = "https://auth.twitter.chanombude.me"
nameservers  = [...]
```

## Set real secret values

`make stage-down` hard-deletes the Secrets Manager entries (`recovery_window_days = 0`), so they come back as `REPLACE_ME` placeholders every cold start. Set the real values:

```bash
aws secretsmanager put-secret-value --region ap-southeast-1 \
  --secret-id twitter-mc-stage/openai-api-key \
  --secret-string "$(grep ^OPENAI_API_KEY .env | cut -d= -f2)"

aws secretsmanager put-secret-value --region ap-southeast-1 \
  --secret-id twitter-mc-stage/service-token \
  --secret-string "$(openssl rand -hex 32)"

aws secretsmanager put-secret-value --region ap-southeast-1 \
  --secret-id twitter-mc-stage/keycloak-admin-password \
  --secret-string "$(openssl rand -base64 24 | tr -d '/+=')"

aws secretsmanager put-secret-value --region ap-southeast-1 \
  --secret-id twitter-mc-stage/keycloak-admin-client-secret \
  --secret-string "$(openssl rand -hex 16)"

# Optional — only if you want Google login on Keycloak
aws secretsmanager put-secret-value --region ap-southeast-1 \
  --secret-id twitter-mc-stage/google-client-id \
  --secret-string "$(grep ^GOOGLE_CLIENT_ID .env | cut -d= -f2)"
aws secretsmanager put-secret-value --region ap-southeast-1 \
  --secret-id twitter-mc-stage/google-client-secret \
  --secret-string "$(grep ^GOOGLE_CLIENT_SECRET .env | cut -d= -f2)"
```

Force External Secrets Operator to re-sync immediately (otherwise it waits up to 1h):

```bash
make stage-kubeconfig                                                           # adds kubectl context once
kubectl --context twitter-mc-stage -n apps delete secret app-secrets
```

ESO recreates `app-secrets` from the new AWS values within seconds.

## Verify the stack

```bash
# All pods Running
kubectl --context twitter-mc-stage -n apps get pods
# Expect: keycloak, kong, web + 6 services, all 1/1 Running

# Public URLs respond
curl -I https://twitter.chanombude.me     # HTTP 307 -> /login
curl -I https://api.twitter.chanombude.me # HTTP 401 (Kong JWT rejecting unauth — expected)
curl -I https://auth.twitter.chanombude.me # HTTP 302 -> Keycloak admin path

# Open the web app
open https://twitter.chanombude.me
# Sign up via the Keycloak realm (Google or email/password). Land back authenticated.
```

## Keycloak admin

```bash
# Get admin password
aws secretsmanager get-secret-value --region ap-southeast-1 \
  --secret-id twitter-mc-stage/keycloak-admin-password \
  --query SecretString --output text

# Login at: https://auth.twitter.chanombude.me/admin
#   username: admin
```

The `twitter` realm + `twitter-app` + `kong-admin` clients are auto-imported on first Keycloak boot from the `keycloak-realm` ConfigMap rendered by terraform. No manual realm setup needed.

## Tear-down

```bash
# Pre-cleanup: delete the Ingress first and let the ALB controller release the ALB
# Skipping this can leave an orphan ALB whose ENIs block VPC destroy.
kubectl --context twitter-mc-stage -n apps delete ingress stage --wait=true
sleep 60

make stage-down               # ~10 min — drops cost to ~$0
```

The bootstrap stack (S3 state + GitHub OIDC role) survives.

If `stage-down` fails partway with `DependencyViolation` on subnets/IGW, the recovery is:
```bash
# Find + delete the orphan ALB
aws elbv2 describe-load-balancers --region ap-southeast-1 \
  --query 'LoadBalancers[?starts_with(LoadBalancerName, `k8s-twitter`)].LoadBalancerArn' --output text \
  | xargs -I{} aws elbv2 delete-load-balancer --region ap-southeast-1 --load-balancer-arn {}
sleep 60
cd infra/terraform/envs/stage && terraform state rm 'module.ingress.kubernetes_ingress_v1.main'
cd ../../.. && make stage-down
```

If `stage-down` then hangs on `module.secrets.kubernetes_namespace.apps: Still destroying...` with a `context deadline exceeded` error, the namespace is stuck because the Ingress/TargetGroupBinding finalizers can't be cleared (their controller is the ALB controller, which has nothing to reconcile against now that the ALB is gone). Force-clear:
```bash
kubectl --context twitter-mc-stage -n apps patch ingress stage \
  -p '{"metadata":{"finalizers":[]}}' --type=merge
for tgb in $(kubectl --context twitter-mc-stage -n apps get targetgroupbindings -o name); do
  kubectl --context twitter-mc-stage -n apps patch "$tgb" \
    -p '{"metadata":{"finalizers":[]}}' --type=merge
done
make stage-down
```

If `stage-down` then errors with `provider["registry.terraform.io/alekc/kubectl"]: invalid provider configuration` near the end, the cluster is already gone but there are k8s resources still in state. The alekc/kubectl provider eagerly evaluates its config and fails on the null cluster endpoint. State-rm everything k8s-related, then retry:
```bash
cd infra/terraform/envs/stage
terraform state list | grep -E "(kubernetes_|kubectl_manifest)" | while read r; do
  terraform state rm "$r"
done
cd ../../.. && make stage-down
```

If `stage-down` finally errors with `DeleteVpc ... DependencyViolation`, two orphan ALB-controller SGs are likely still attached. They got auto-created at Ingress deploy time and aren't in terraform state. Delete them manually:
```bash
aws ec2 describe-security-groups --region ap-southeast-1 \
  --filters "Name=vpc-id,Values=$(terraform -chdir=infra/terraform/envs/stage output -raw vpc_id 2>/dev/null || echo SET-MANUALLY)" \
  --query 'SecurityGroups[?GroupName!=`default` && starts_with(GroupName, `k8s-`)].GroupId' --output text \
  | tr '\t' '\n' \
  | xargs -I{} aws ec2 delete-security-group --region ap-southeast-1 --group-id {}
make stage-down
```

## Cost reference

| State | $/day |
|---|---|
| Bootstrap only (after `stage-down`) | ~$0 |
| Full stack running | ~$10-11 |

See `infra/terraform/` README or [docs/ARCHITECTURE.md](ARCHITECTURE.md) for the per-component breakdown.

## Troubleshooting

| Symptom | Fix |
|---|---|
| `Error acquiring the state lock` | `make stage-unlock` (only if you're sure no other terraform is running) |
| ACM cert hangs for >30 min | NS records at registrar didn't propagate — check `dig @8.8.8.8 NS twitter.chanombude.me +short` |
| Keycloak admin UI shows `somethingWentWrong` | Pod proxy config issue — `kubectl logs` to check for "Hostname v1 options [proxy]" warning |
| Pod `CrashLoopBackOff` after secret rotation | `kubectl delete pod -l app.kubernetes.io/name=<svc>` to pick up the new env from ESO-synced Secret |
| Realm changes don't apply | `--import-realm` skips existing realms. Delete the `twitter` realm via admin UI, then `kubectl delete pod -l app.kubernetes.io/name=keycloak` |

For deeper failure modes, the memory at `.claude/projects/.../memory/feedback_terraform_cluster_first_apply.md` has every gotcha we hit during Phase 9 build-out.
