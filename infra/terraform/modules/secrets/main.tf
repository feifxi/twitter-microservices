# Secrets Manager — placeholder entries.
# terraform creates each secret with REPLACE_ME, then ignore_changes leaves the
# value alone forever. You set the real value once in the AWS console; ESO syncs it.

locals {
  placeholder_secrets = toset([
    "openai-api-key",
    "service-token",
    "google-client-id",
    "google-client-secret",
    "keycloak-admin-password",
    "keycloak-admin-client-secret",
  ])
}

resource "aws_secretsmanager_secret" "placeholder" {
  for_each = local.placeholder_secrets

  name                    = "${var.name_prefix}/${each.value}"
  recovery_window_in_days = var.recovery_window_days
  tags                    = var.tags
}

resource "aws_secretsmanager_secret_version" "placeholder_initial" {
  for_each = aws_secretsmanager_secret.placeholder

  secret_id     = each.value.id
  secret_string = "REPLACE_ME"

  lifecycle {
    # User sets the real value in the AWS console after first apply.
    # terraform never overwrites it on subsequent applies.
    ignore_changes = [secret_string, version_stages]
  }
}

resource "kubernetes_namespace" "apps" {
  metadata {
    name = var.apps_namespace
  }
}

# One ClusterSecretStore for everything. Authenticates as the external-secrets
# ServiceAccount, whose IRSA role grants SecretsManager:GetSecretValue.
resource "kubectl_manifest" "secret_store" {
  yaml_body = yamlencode({
    apiVersion = "external-secrets.io/v1beta1"
    kind       = "ClusterSecretStore"
    metadata   = { name = "aws-secrets" }
    spec = {
      provider = {
        aws = {
          service = "SecretsManager"
          region  = var.region
          auth = {
            jwt = {
              serviceAccountRef = {
                name      = "external-secrets"
                namespace = "external-secrets"
              }
            }
          }
        }
      }
    }
  })
}

# All 6 placeholder secrets land in one k8s Secret named "app-secrets".
# Chunk 7 mounts this as envFrom.secretRef into every pod that needs them.
resource "kubectl_manifest" "app_secrets" {
  yaml_body = yamlencode({
    apiVersion = "external-secrets.io/v1beta1"
    kind       = "ExternalSecret"
    metadata = {
      name      = "app-secrets"
      namespace = var.apps_namespace
    }
    spec = {
      refreshInterval = "1h"
      secretStoreRef = {
        name = "aws-secrets"
        kind = "ClusterSecretStore"
      }
      target = {
        name           = "app-secrets"
        creationPolicy = "Owner"
      }
      data = [
        for s in local.placeholder_secrets : {
          secretKey = replace(upper(s), "-", "_")
          remoteRef = {
            key = "${var.name_prefix}/${s}"
          }
        }
      ]
    }
  })

  depends_on = [
    kubectl_manifest.secret_store,
    aws_secretsmanager_secret_version.placeholder_initial,
    kubernetes_namespace.apps,
  ]
}

# Aurora credentials come from the RDS-managed JSON secret {username, password}.
# Split into discrete keys so service env vars consume DATABASE_USERNAME / DATABASE_PASSWORD directly.
resource "kubectl_manifest" "aurora_credentials" {
  yaml_body = yamlencode({
    apiVersion = "external-secrets.io/v1beta1"
    kind       = "ExternalSecret"
    metadata = {
      name      = "aurora-credentials"
      namespace = var.apps_namespace
    }
    spec = {
      refreshInterval = "1h"
      secretStoreRef = {
        name = "aws-secrets"
        kind = "ClusterSecretStore"
      }
      target = {
        name           = "aurora-credentials"
        creationPolicy = "Owner"
      }
      data = [
        {
          secretKey = "DATABASE_USERNAME"
          remoteRef = {
            key      = var.aurora_master_secret_arn
            property = "username"
          }
        },
        {
          secretKey = "DATABASE_PASSWORD"
          remoteRef = {
            key      = var.aurora_master_secret_arn
            property = "password"
          }
        },
      ]
    }
  })

  depends_on = [
    kubectl_manifest.secret_store,
    kubernetes_namespace.apps,
  ]
}
