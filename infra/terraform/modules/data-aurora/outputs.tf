output "cluster_endpoint" {
  description = "Writer endpoint. Use this in service DATABASE_URL."
  value       = aws_rds_cluster.this.endpoint
}

output "reader_endpoint" {
  value = aws_rds_cluster.this.reader_endpoint
}

output "port" {
  value = aws_rds_cluster.this.port
}

output "database_name" {
  value = aws_rds_cluster.this.database_name
}

output "master_user_secret_arn" {
  description = "ARN of the RDS-managed Secrets Manager secret holding {username,password}. ExternalSecret reads from this."
  value       = aws_rds_cluster.this.master_user_secret[0].secret_arn
}

output "security_group_id" {
  description = "Aurora SG. Narrow to EKS-node SG only once nodes exist (currently VPC-CIDR wide)."
  value       = aws_security_group.this.id
}
