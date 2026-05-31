locals {
  services = {
    "user-service" = {
      port      = 8001
      grpc_port = 9090
      db_schema = "users"
      extras = [
        { name = "KEYCLOAK_BASE_URL", value = var.keycloak_internal_url },
        { name = "KEYCLOAK_REALM", value = "twitter" },
        { name = "KEYCLOAK_ADMIN_CLIENT_ID", value = "admin-cli" },
        { name = "MIGRATIONS_PATH", value = "file://migrations" },
        { name = "MIGRATIONS_TABLE", value = "user_migrations" },
      ]
      from_app_secrets = ["KEYCLOAK_ADMIN_CLIENT_SECRET", "KEYCLOAK_ADMIN_PASSWORD"]
    }
    "tweet-service" = {
      port      = 8002
      grpc_port = 9091
      db_schema = "tweet"
      extras = [
        { name = "USER_SERVICE_GRPC_ADDR", value = "user-service:9090" },
        { name = "MIGRATIONS_PATH", value = "file://migrations" },
        { name = "MIGRATIONS_TABLE", value = "tweet_migrations" },
      ]
      from_app_secrets = []
    }
    "feed-service" = {
      port      = 8003
      grpc_port = null
      db_schema = null
      extras = [
        { name = "USER_SERVICE_GRPC_ADDR", value = "user-service:9090" },
        { name = "TWEET_SERVICE_GRPC_ADDR", value = "tweet-service:9091" },
      ]
      from_app_secrets = []
    }
    "notification-service" = {
      port      = 8004
      grpc_port = null
      db_schema = "notification"
      extras = [
        { name = "MIGRATIONS_PATH", value = "file://migrations" },
        { name = "MIGRATIONS_TABLE", value = "notification_migrations" },
      ]
      from_app_secrets = []
    }
    "media-service" = {
      port      = 8005
      grpc_port = null
      db_schema = null
      irsa      = var.media_irsa_role_arn
      extras = [
        { name = "S3_BUCKET", value = var.media_s3_bucket },
        { name = "MEDIA_PUBLIC_URL_BASE", value = var.media_s3_public_url_base },
      ]
      from_app_secrets = []
    }
    "search-service" = {
      port      = 8006
      grpc_port = null
      db_schema = null
      extras = [
        { name = "OPENSEARCH_URL", value = var.opensearch_endpoint },
        { name = "OPENAI_EMBEDDING_MODEL", value = "text-embedding-3-small" },
        { name = "USER_SERVICE_GRPC_ADDR", value = "user-service:9090" },
        { name = "TWEET_SERVICE_GRPC_ADDR", value = "tweet-service:9091" },
      ]
      from_app_secrets = ["OPENAI_API_KEY"]
    }
    "web" = {
      port      = 3000
      grpc_port = null
      db_schema = null
      extras = [
        { name = "KEYCLOAK_URL", value = "https://auth.${var.domain}" },
        { name = "KEYCLOAK_REALM", value = "twitter" },
        { name = "KEYCLOAK_CLIENT_ID", value = "twitter-app" },
        { name = "APP_URL", value = "https://${var.domain}" },
        { name = "KONG_URL", value = "http://kong:8000" },
      ]
      from_app_secrets = []
    }
  }

  # Services that need a DATABASE_URL. The aurora-urls ExternalSecret templates
  # one key per service so each Pod can `valueFrom.secretKeyRef.key = "<svc>"`.
  db_services = { for k, v in local.services : k => v if v.db_schema != null }

  labels = { for k, v in var.tags : lower(k) => v }
}

# aurora-urls: ExternalSecret with one DATABASE_URL per DB-using service,
# templated from the RDS-managed {username, password} secret.
resource "kubectl_manifest" "aurora_urls" {
  yaml_body = yamlencode({
    apiVersion = "external-secrets.io/v1beta1"
    kind       = "ExternalSecret"
    metadata = {
      name      = "aurora-urls"
      namespace = var.namespace
    }
    spec = {
      refreshInterval = "1h"
      secretStoreRef = {
        name = "aws-secrets"
        kind = "ClusterSecretStore"
      }
      target = {
        name           = "aurora-urls"
        creationPolicy = "Owner"
        template = {
          engineVersion = "v2"
          data = {
            for svc_name, svc in local.db_services :
            # urlquery percent-encodes the password so special chars
            # (# ( ) * etc. in RDS-managed passwords) don't break URL parsing.
            svc_name => "postgres://{{ .username }}:{{ .password | urlquery }}@${var.aurora_endpoint}/${var.aurora_database}?search_path=${svc.db_schema}&sslmode=require"
          }
        }
      }
      dataFrom = [{
        extract = {
          # ARN of the RDS-managed secret (rds!cluster-<random-suffix>). Holds
          # JSON {username, password, engine, host, port, dbname, ...}.
          # Templating above pulls .username and .password from this extract.
          key = var.aurora_master_secret_arn
        }
      }]
    }
  })
}

# ServiceAccount per service. Annotated with IRSA role if the service has one
# (currently only media-service, which needs S3 access).
resource "kubernetes_service_account" "service" {
  for_each = local.services

  metadata {
    name      = each.key
    namespace = var.namespace
    annotations = lookup(each.value, "irsa", null) != null ? {
      "eks.amazonaws.com/role-arn" = each.value.irsa
    } : {}
  }
}

resource "kubernetes_deployment" "service" {
  for_each = local.services

  metadata {
    name      = each.key
    namespace = var.namespace
    labels = merge(local.labels, {
      "app.kubernetes.io/name" = each.key
    })
  }

  spec {
    replicas = 1

    selector {
      match_labels = { "app.kubernetes.io/name" = each.key }
    }

    template {
      metadata {
        labels = { "app.kubernetes.io/name" = each.key }
        # When the aurora-urls template changes (e.g. password rotation, schema
        # change, encoding fix), this checksum changes and k8s rolls the pod
        # so it picks up the new env from the regenerated secret.
        # nonsensitive(): yaml_body references the Aurora secret ARN (sensitive
        # output), but the sha1 hash exposes nothing.
        annotations = each.value.db_schema != null ? {
          "checksum/aurora-urls" = nonsensitive(sha1(kubectl_manifest.aurora_urls.yaml_body))
        } : {}
      }

      spec {
        service_account_name = kubernetes_service_account.service[each.key].metadata[0].name

        container {
          name              = each.key
          image             = "${var.ecr_urls[each.key]}:${var.image_tag}"
          image_pull_policy = "Always"

          port {
            name           = "http"
            container_port = each.value.port
          }

          dynamic "port" {
            for_each = each.value.grpc_port != null ? [each.value.grpc_port] : []
            content {
              name           = "grpc"
              container_port = port.value
            }
          }

          # Common env to every service
          env {
            name  = "PORT"
            value = tostring(each.value.port)
          }
          env {
            name  = "REDIS_URL"
            value = var.redis_endpoint
          }
          env {
            name  = "KAFKA_BROKERS"
            value = var.kafka_brokers
          }

          # gRPC port (services that listen on gRPC)
          dynamic "env" {
            for_each = each.value.grpc_port != null ? [each.value.grpc_port] : []
            content {
              name  = "GRPC_PORT"
              value = tostring(env.value)
            }
          }

          # DATABASE_URL (services with a schema)
          dynamic "env" {
            for_each = each.value.db_schema != null ? [each.value.db_schema] : []
            content {
              name = "DATABASE_URL"
              value_from {
                secret_key_ref {
                  name = "aurora-urls"
                  key  = each.key
                }
              }
            }
          }

          # SERVICE_TOKEN — every service uses it for service-to-service auth
          env {
            name = "SERVICE_TOKEN"
            value_from {
              secret_key_ref {
                name = var.app_secrets_secret
                key  = "SERVICE_TOKEN"
              }
            }
          }

          # Extra literal env per service
          dynamic "env" {
            for_each = each.value.extras
            content {
              name  = env.value.name
              value = env.value.value
            }
          }

          # Extra env from app-secrets (e.g. KEYCLOAK_ADMIN_PASSWORD, OPENAI_API_KEY)
          dynamic "env" {
            for_each = each.value.from_app_secrets
            content {
              name = env.value
              value_from {
                secret_key_ref {
                  name = var.app_secrets_secret
                  key  = env.value
                }
              }
            }
          }

          resources {
            requests = { cpu = "50m", memory = "128Mi" }
            limits   = { cpu = "500m", memory = "512Mi" }
          }

          readiness_probe {
            http_get {
              path = each.key == "web" ? "/" : "/healthz"
              port = each.value.port
            }
            initial_delay_seconds = 10
            period_seconds        = 10
          }

          liveness_probe {
            http_get {
              path = each.key == "web" ? "/" : "/livez"
              port = each.value.port
            }
            initial_delay_seconds = 30
            period_seconds        = 30
          }
        }
      }
    }
  }

  depends_on = [kubectl_manifest.aurora_urls]
}

resource "kubernetes_service" "service" {
  for_each = local.services

  metadata {
    name      = each.key
    namespace = var.namespace
    labels    = { "app.kubernetes.io/name" = each.key }
  }

  spec {
    type     = "ClusterIP"
    selector = { "app.kubernetes.io/name" = each.key }

    port {
      name        = "http"
      port        = each.value.port
      target_port = each.value.port
    }

    dynamic "port" {
      for_each = each.value.grpc_port != null ? [each.value.grpc_port] : []
      content {
        name        = "grpc"
        port        = port.value
        target_port = port.value
      }
    }
  }
}
