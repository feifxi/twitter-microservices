output "service_name" {
  description = "ClusterIP service name. The Ingress routes api.<domain> here on port 8000."
  value       = kubernetes_service.kong.metadata[0].name
}

output "internal_url" {
  description = "Cluster-internal Kong URL. Web app uses this as KONG_URL for SSR-side API proxying."
  value       = "http://${kubernetes_service.kong.metadata[0].name}.${kubernetes_service.kong.metadata[0].namespace}.svc.cluster.local:8000"
}
