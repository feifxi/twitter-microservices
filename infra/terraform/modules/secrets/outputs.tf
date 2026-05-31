output "apps_namespace" {
  description = "Kubernetes namespace where application workloads run."
  value       = kubernetes_namespace.apps.metadata[0].name
}

output "placeholder_secret_names" {
  description = "Map of short-name -> AWS Secrets Manager full name. Useful for `aws secretsmanager put-secret-value` after first apply."
  value       = { for n, s in aws_secretsmanager_secret.placeholder : n => s.name }
}
