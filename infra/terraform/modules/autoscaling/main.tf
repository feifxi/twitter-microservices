resource "kubernetes_horizontal_pod_autoscaler_v2" "this" {
  for_each = var.hpa_targets

  metadata {
    name      = each.key
    namespace = var.namespace
  }

  spec {
    scale_target_ref {
      api_version = "apps/v1"
      kind        = "Deployment"
      name        = each.key
    }

    min_replicas = each.value.min_replicas
    max_replicas = each.value.max_replicas

    metric {
      type = "Resource"
      resource {
        name = "cpu"
        target {
          type                = "Utilization"
          average_utilization = each.value.cpu_target_pct
        }
      }
    }

    # Default scale-up window is 0s (instant) which causes thrash; pin at
    # 60s. Scale-down stays at the k8s default 300s — better to over-provision
    # briefly than to flap during traffic dips.
    behavior {
      scale_up {
        stabilization_window_seconds = 60
        policy {
          type           = "Percent"
          value          = 100
          period_seconds = 60
        }
      }
    }
  }
}
