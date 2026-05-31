locals {
  labels = merge({ "app.kubernetes.io/name" = "kong" }, { for k, v in var.tags : lower(k) => v })

  # Stock Kong's jwt plugin validates the token but doesn't map claims to
  # headers — services expect X-User-ID / X-User-Email / X-User-Username.
  # This Lua script (run via pre-function plugin AFTER jwt validation) decodes
  # the JWT payload and injects the claims as headers. Identical to local dev.
  lua_inject_user_headers = <<-EOT
    local auth = kong.request.get_header("Authorization")
    if not auth then return end
    local token = auth:match("^Bearer%s+(.+)$")
    if not token then return end
    local payload_b64 = token:match("^[^%.]+%.([^%.]+)%.[^%.]+$")
    if not payload_b64 then return end
    local b64 = payload_b64:gsub("%-", "+"):gsub("_", "/")
    local pad = #b64 % 4
    if pad == 2 then b64 = b64 .. "=="
    elseif pad == 3 then b64 = b64 .. "=" end
    local decoded = ngx.decode_base64(b64)
    if not decoded then return end
    local function claim(json, key)
      return json:match('"' .. key .. '"%s*:%s*"([^"]*)"')
    end
    local sub = claim(decoded, "sub")
    local email = claim(decoded, "email")
    local username = claim(decoded, "preferred_username")
    if sub then kong.service.request.set_header("X-User-ID", sub) end
    if email then kong.service.request.set_header("X-User-Email", email) end
    if username then kong.service.request.set_header("X-User-Username", username) end
  EOT

  route_plugins = [
    {
      name = "jwt"
      config = {
        key_claim_name   = "iss"
        claims_to_verify = ["exp"]
      }
    },
    {
      name = "pre-function"
      config = {
        access = [local.lua_inject_user_headers]
      }
    },
  ]

  kong_config = {
    _format_version = "3.0"

    # One consumer holding the realm's public key. Kong matches the JWT's `iss`
    # claim to this consumer's jwt_secret.key, then validates the signature with
    # rsa_public_key. No JWKS round-trip, no per-user consumer setup.
    consumers = [
      {
        username = "keycloak"
        jwt_secrets = [
          {
            key            = var.realm_issuer
            algorithm      = "RS256"
            secret         = "unused-for-rsa256" # Required by Kong schema; ignored for RS256
            rsa_public_key = var.realm_public_key_pem
          }
        ]
      }
    ]

    services = [
      for name, cfg in var.routes : {
        name = name
        url  = "http://${name}.${var.namespace}.svc.cluster.local:${cfg.port}"
        routes = [{
          name       = "${name}-route"
          paths      = cfg.paths
          strip_path = false
          plugins    = local.route_plugins
        }]
      }
    ]
  }
}

resource "kubernetes_config_map" "kong" {
  metadata {
    name      = "kong-config"
    namespace = var.namespace
  }

  data = {
    "kong.yml" = yamlencode(local.kong_config)
  }
}

resource "kubernetes_deployment" "kong" {
  metadata {
    name      = "kong"
    namespace = var.namespace
    labels    = local.labels
  }

  spec {
    replicas = var.replicas

    selector {
      match_labels = { "app.kubernetes.io/name" = "kong" }
    }

    template {
      metadata {
        labels = local.labels
        # Rolling restart when the config changes.
        # nonsensitive(): kong.yml contains the realm public key (potentially
        # marked sensitive through the module-output chain); the sha1 leaks nothing.
        annotations = {
          "checksum/config" = nonsensitive(sha1(kubernetes_config_map.kong.data["kong.yml"]))
        }
      }

      spec {
        container {
          name  = "kong"
          image = var.image

          # DB-less mode, declarative config from the mounted ConfigMap.
          env {
            name  = "KONG_DATABASE"
            value = "off"
          }
          env {
            name  = "KONG_DECLARATIVE_CONFIG"
            value = "/kong-config/kong.yml"
          }
          env {
            name  = "KONG_PROXY_LISTEN"
            value = "0.0.0.0:8000"
          }
          # Admin API not needed in DB-less mode; turn it off so the port can't be reached.
          env {
            name  = "KONG_ADMIN_LISTEN"
            value = "off"
          }
          # Status endpoint for probes.
          env {
            name  = "KONG_STATUS_LISTEN"
            value = "0.0.0.0:8100"
          }
          env {
            name  = "KONG_PROXY_ACCESS_LOG"
            value = "/dev/stdout"
          }
          env {
            name  = "KONG_PROXY_ERROR_LOG"
            value = "/dev/stderr"
          }

          port {
            name           = "proxy"
            container_port = 8000
          }
          port {
            name           = "status"
            container_port = 8100
          }

          volume_mount {
            name       = "config"
            mount_path = "/kong-config"
            read_only  = true
          }

          resources {
            requests = { cpu = "50m", memory = "128Mi" }
            limits   = { cpu = "500m", memory = "512Mi" }
          }

          readiness_probe {
            http_get {
              path = "/status"
              port = 8100
            }
            initial_delay_seconds = 5
            period_seconds        = 5
          }

          liveness_probe {
            http_get {
              path = "/status"
              port = 8100
            }
            initial_delay_seconds = 15
            period_seconds        = 30
          }
        }

        volume {
          name = "config"
          config_map {
            name = kubernetes_config_map.kong.metadata[0].name
          }
        }
      }
    }
  }
}

resource "kubernetes_service" "kong" {
  metadata {
    name      = "kong"
    namespace = var.namespace
    labels    = local.labels
  }

  spec {
    type     = "ClusterIP"
    selector = { "app.kubernetes.io/name" = "kong" }

    port {
      name        = "proxy"
      port        = 8000
      target_port = 8000
    }
  }
}
