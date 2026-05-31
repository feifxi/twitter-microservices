output "bootstrap_brokers" {
  description = "Comma-separated PLAINTEXT bootstrap server list. Use this as KAFKA_BROKERS in service env."
  value       = aws_msk_cluster.this.bootstrap_brokers
}

output "bootstrap_brokers_tls" {
  description = "TLS bootstrap server list — for the future hardening when Go clients learn TLS."
  value       = aws_msk_cluster.this.bootstrap_brokers_tls
}

output "cluster_arn" {
  value = aws_msk_cluster.this.arn
}

output "security_group_id" {
  value = aws_security_group.this.id
}
