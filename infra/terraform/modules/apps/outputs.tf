output "service_names" {
  description = "k8s Service names per service. Kong proxies to <name>:<port>.<namespace>.svc.cluster.local."
  value       = { for k, s in kubernetes_service.service : k => s.metadata[0].name }
}

output "web_service_name" {
  value = kubernetes_service.service["web"].metadata[0].name
}
