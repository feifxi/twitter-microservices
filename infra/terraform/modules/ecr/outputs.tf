output "repository_urls" {
  description = "Map of repo short-name to repository_url (host/repo). Use for docker tag/push and as image in Helm values."
  value       = { for n, r in aws_ecr_repository.this : n => r.repository_url }
}

output "repository_arns" {
  value = { for n, r in aws_ecr_repository.this : n => r.arn }
}

output "registry_id" {
  description = "ECR registry account id; same as caller account but exposed for clarity."
  value       = values(aws_ecr_repository.this)[0].registry_id
}
