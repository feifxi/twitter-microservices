output "endpoint" {
  description = "HTTPS URL for OPENSEARCH_URL. No port — AWS uses 443."
  value       = "https://${aws_opensearch_domain.this.endpoint}"
}

output "domain_name" {
  value = aws_opensearch_domain.this.domain_name
}

output "security_group_id" {
  value = aws_security_group.this.id
}
