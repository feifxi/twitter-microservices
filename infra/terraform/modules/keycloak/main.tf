locals {
  labels = merge({
    "app.kubernetes.io/name"      = "keycloak"
    "app.kubernetes.io/component" = "auth"
  }, { for k, v in var.tags : lower(k) => v })
}

# RSA key pair for the realm. Baked into the realm import so the public key is
# known up-front (Kong's JWT plugin validates against it without round-tripping
# to JWKS). Same key survives stage-down/up cycles because it's in terraform state.

resource "tls_private_key" "realm" {
  algorithm = "RSA"
  rsa_bits  = 2048
}

resource "tls_self_signed_cert" "realm" {
  private_key_pem = tls_private_key.realm.private_key_pem

  subject {
    common_name = "keycloak-${var.namespace}"
  }

  validity_period_hours = 24 * 365 * 10 # 10 years; rotate via `terraform apply -replace`
  early_renewal_hours   = 24 * 30
  allowed_uses          = ["digital_signature", "key_encipherment", "server_auth"]
}

# Keycloak's import expects raw base64 of the DER bytes (no PEM headers).
locals {
  private_key_b64 = replace(
    replace(
      replace(tls_private_key.realm.private_key_pem_pkcs8, "-----BEGIN PRIVATE KEY-----", ""),
      "-----END PRIVATE KEY-----", ""
    ), "\n", ""
  )
  certificate_b64 = replace(
    replace(
      replace(tls_self_signed_cert.realm.cert_pem, "-----BEGIN CERTIFICATE-----", ""),
      "-----END CERTIFICATE-----", ""
    ), "\n", ""
  )

  realm_json = templatefile("${path.module}/realm-twitter.json.tpl", {
    domain          = var.web_domain
    private_key_b64 = local.private_key_b64
    certificate_b64 = local.certificate_b64
  })
}

# ConfigMap containing the rendered realm. Mounted at /opt/keycloak/data/import
# where Keycloak picks it up when started with --import-realm.
resource "kubernetes_config_map" "realm" {
  metadata {
    name      = "keycloak-realm"
    namespace = var.namespace
  }

  data = {
    "realm-twitter.json" = local.realm_json
  }
}

resource "kubernetes_deployment" "keycloak" {
  metadata {
    name      = "keycloak"
    namespace = var.namespace
    labels    = local.labels
  }

  spec {
    replicas = var.replicas

    selector {
      match_labels = { "app.kubernetes.io/name" = "keycloak" }
    }

    template {
      metadata {
        labels = local.labels
        # Rolling restart when the realm content changes (key rotation, redirect
        # URI changes, new clients) — Keycloak re-imports on boot.
        # nonsensitive(): realm_json contains the RSA private key marked
        # sensitive, but the sha1 hash leaks nothing; terraform won't put a
        # sensitive-tainted value into a plain annotation without this wrap.
        annotations = {
          "checksum/realm" = nonsensitive(sha1(local.realm_json))
        }
      }

      spec {
        container {
          name              = "keycloak"
          image             = "${var.image}:${var.image_tag}"
          image_pull_policy = "Always"

          # --import-realm processes any *.json files in /opt/keycloak/data/import on boot.
          # Idempotent: if realm already exists, contents are merged unless overwrite is set.
          args = ["start", "--optimized", "--import-realm"]

          port {
            name           = "http"
            container_port = 8080
          }
          port {
            name           = "management"
            container_port = 9000
          }

          env {
            name  = "KC_DB"
            value = "postgres"
          }
          env {
            name  = "KC_DB_URL"
            value = "jdbc:postgresql://${var.aurora_endpoint}/${var.aurora_database}"
          }
          env {
            name  = "KC_HOSTNAME"
            value = "https://${var.hostname}"
          }
          # Keycloak 26 deprecated KC_PROXY=edge. The new way: tell Keycloak to
          # trust X-Forwarded-* headers (which ALB sends), so it generates HTTPS
          # redirect URLs and resolves the right scheme/host.
          env {
            name  = "KC_PROXY_HEADERS"
            value = "xforwarded"
          }
          env {
            name  = "KC_HTTP_ENABLED"
            value = "true"
          }
          env {
            name  = "KC_HEALTH_ENABLED"
            value = "true"
          }
          env {
            name  = "KEYCLOAK_ADMIN"
            value = "admin"
          }

          env {
            name = "KC_DB_USERNAME"
            value_from {
              secret_key_ref {
                name = var.aurora_credentials_secret
                key  = "DATABASE_USERNAME"
              }
            }
          }
          env {
            name = "KC_DB_PASSWORD"
            value_from {
              secret_key_ref {
                name = var.aurora_credentials_secret
                key  = "DATABASE_PASSWORD"
              }
            }
          }

          env {
            name = "KEYCLOAK_ADMIN_PASSWORD"
            value_from {
              secret_key_ref {
                name = var.app_secrets_secret
                key  = "KEYCLOAK_ADMIN_PASSWORD"
              }
            }
          }

          # The realm.json uses ${env.KC_ADMIN_CLIENT_SECRET} as the kong-admin
          # client secret. Sourced from the same Secrets Manager entry user-service uses.
          env {
            name = "KC_ADMIN_CLIENT_SECRET"
            value_from {
              secret_key_ref {
                name = var.app_secrets_secret
                key  = "KEYCLOAK_ADMIN_CLIENT_SECRET"
              }
            }
          }

          # Google OIDC identity provider credentials. realm.json substitutes
          # ${env.KC_GOOGLE_*} at import time. If the secret values are still
          # the REPLACE_ME placeholder, the Google login button shows but
          # clicking it fails — set real values in Secrets Manager when you
          # want Google login functional.
          env {
            name = "KC_GOOGLE_CLIENT_ID"
            value_from {
              secret_key_ref {
                name = var.app_secrets_secret
                key  = "GOOGLE_CLIENT_ID"
              }
            }
          }
          env {
            name = "KC_GOOGLE_CLIENT_SECRET"
            value_from {
              secret_key_ref {
                name = var.app_secrets_secret
                key  = "GOOGLE_CLIENT_SECRET"
              }
            }
          }

          volume_mount {
            name       = "realm-import"
            mount_path = "/opt/keycloak/data/import"
            read_only  = true
          }

          resources {
            requests = var.resources.requests
            limits   = var.resources.limits
          }

          startup_probe {
            http_get {
              path = "/health/started"
              port = 9000
            }
            failure_threshold     = 30
            period_seconds        = 5
            initial_delay_seconds = 15
          }
          readiness_probe {
            http_get {
              path = "/health/ready"
              port = 9000
            }
            period_seconds = 10
          }
          liveness_probe {
            http_get {
              path = "/health/live"
              port = 9000
            }
            period_seconds = 30
          }
        }

        volume {
          name = "realm-import"
          config_map {
            name = kubernetes_config_map.realm.metadata[0].name
          }
        }
      }
    }
  }
}

resource "kubernetes_service" "keycloak" {
  metadata {
    name      = "keycloak"
    namespace = var.namespace
    labels    = local.labels
  }

  spec {
    type     = "ClusterIP"
    selector = { "app.kubernetes.io/name" = "keycloak" }

    port {
      name        = "http"
      port        = 8080
      target_port = 8080
    }
  }
}
