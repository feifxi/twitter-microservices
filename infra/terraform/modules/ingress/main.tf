locals {
  web_host     = var.domain
  api_host     = "api.${var.domain}"
  auth_host    = "auth.${var.domain}"
  ingress_name = "stage"
  group_name   = "twitter-stage"
  alb_tags_csv = join(",", [for k, v in var.tags : "${k}=${v}"])
}

# One Ingress with three host rules. The AWS Load Balancer Controller turns this
# into a single ALB with host-based routing. The group.name annotation lets future
# Ingresses in this namespace share the same ALB (zero extra cost).
resource "kubernetes_ingress_v1" "main" {
  metadata {
    name      = local.ingress_name
    namespace = var.namespace
    annotations = {
      "alb.ingress.kubernetes.io/scheme"           = "internet-facing"
      "alb.ingress.kubernetes.io/target-type"      = "ip"
      "alb.ingress.kubernetes.io/listen-ports"     = jsonencode([{ HTTP = 80 }, { HTTPS = 443 }])
      "alb.ingress.kubernetes.io/ssl-redirect"     = "443"
      "alb.ingress.kubernetes.io/certificate-arn"  = var.certificate_arn
      "alb.ingress.kubernetes.io/group.name"       = local.group_name
      "alb.ingress.kubernetes.io/healthcheck-path" = "/"
      # SSE requires a longer idle timeout. Default 60s drops streams; 300s
      # matches the notification-service SSE behaviour from local dev.
      "alb.ingress.kubernetes.io/load-balancer-attributes" = "idle_timeout.timeout_seconds=300"
      "alb.ingress.kubernetes.io/tags"                     = local.alb_tags_csv
    }
  }

  spec {
    ingress_class_name = "alb"

    rule {
      host = local.web_host
      http {
        path {
          path      = "/"
          path_type = "Prefix"
          backend {
            service {
              name = var.web_service_name
              port { number = var.web_service_port }
            }
          }
        }
      }
    }

    rule {
      host = local.api_host
      http {
        path {
          path      = "/"
          path_type = "Prefix"
          backend {
            service {
              name = var.kong_service_name
              port { number = var.kong_service_port }
            }
          }
        }
      }
    }

    rule {
      host = local.auth_host
      http {
        path {
          path      = "/"
          path_type = "Prefix"
          backend {
            service {
              name = var.keycloak_service_name
              port { number = var.keycloak_service_port }
            }
          }
        }
      }
    }
  }

  # Block apply until the ALB controller assigns a hostname. Required so the
  # Route53 A-ALIAS records below can resolve the ALB.
  wait_for_load_balancer = true
}

# The aws_route53_record ALIAS block needs the ALB's canonical zone id, which
# isn't returned by the Ingress status. Tag-based lookup is more robust than
# parsing the hostname — the AWS Load Balancer Controller stamps the ingress
# group name onto the ALB so we can find it without string surgery.
data "aws_lb" "ingress" {
  tags = {
    "ingress.k8s.aws/stack" = local.group_name
  }

  depends_on = [kubernetes_ingress_v1.main]
}

resource "aws_route53_record" "web" {
  zone_id = var.zone_id
  name    = local.web_host
  type    = "A"

  alias {
    name                   = kubernetes_ingress_v1.main.status[0].load_balancer[0].ingress[0].hostname
    zone_id                = data.aws_lb.ingress.zone_id
    evaluate_target_health = true
  }
}

resource "aws_route53_record" "api" {
  zone_id = var.zone_id
  name    = local.api_host
  type    = "A"

  alias {
    name                   = kubernetes_ingress_v1.main.status[0].load_balancer[0].ingress[0].hostname
    zone_id                = data.aws_lb.ingress.zone_id
    evaluate_target_health = true
  }
}

resource "aws_route53_record" "auth" {
  zone_id = var.zone_id
  name    = local.auth_host
  type    = "A"

  alias {
    name                   = kubernetes_ingress_v1.main.status[0].load_balancer[0].ingress[0].hostname
    zone_id                = data.aws_lb.ingress.zone_id
    evaluate_target_health = true
  }
}
