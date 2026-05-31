output "hpa_names" {
  description = "Map of service -> HPA resource name. Inspect with `kubectl describe hpa <name>` to see live utilisation."
  value       = { for k, h in kubernetes_horizontal_pod_autoscaler_v2.this : k => h.metadata[0].name }
}
