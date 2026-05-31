output "endpoint" {
  description = "host:port string for REDIS_URL env var. cache_nodes[0] is the only node on a single-node cluster."
  value       = "${aws_elasticache_cluster.this.cache_nodes[0].address}:${aws_elasticache_cluster.this.cache_nodes[0].port}"
}

output "security_group_id" {
  value = aws_security_group.this.id
}
