output "vpc_id" {
  value = module.vpc.vpc_id
}

output "private_subnet_ids" {
  value = module.vpc.private_subnet_ids
}

output "public_subnet_ids" {
  value = module.vpc.public_subnet_ids
}

output "aurora_endpoint" {
  value = module.aurora.cluster_endpoint
}

output "aurora_database_name" {
  value = module.aurora.database_name
}

output "aurora_master_secret_arn" {
  value     = module.aurora.master_user_secret_arn
  sensitive = true
}

output "redis_endpoint" {
  value = module.elasticache.endpoint
}

output "kafka_bootstrap_brokers" {
  value     = module.msk.bootstrap_brokers
  sensitive = true
}

output "opensearch_endpoint" {
  value = module.opensearch.endpoint
}

output "eks_cluster_name" {
  value = module.eks_cluster.cluster_name
}

output "eks_cluster_endpoint" {
  value = module.eks_cluster.cluster_endpoint
}

output "eks_node_security_group_id" {
  value = module.eks_cluster.node_security_group_id
}

output "ecr_repository_urls" {
  description = "Map of short-name -> ECR URL. Used for docker tag/push and as image= in service deployments."
  value       = module.ecr.repository_urls
}

output "secret_names" {
  description = "AWS Secrets Manager paths to update via console once after first apply."
  value       = module.secrets.placeholder_secret_names
}

output "apps_namespace" {
  value = module.secrets.apps_namespace
}

output "keycloak_internal_url" {
  description = "Cluster-internal Keycloak URL. user-service calls this for admin API; Kong calls it for JWKS."
  value       = module.keycloak.internal_url
}

output "keycloak_jwks_url" {
  description = "JWKS endpoint Kong uses to validate JWTs."
  value       = module.keycloak.jwks_url
}

output "media_s3_bucket" {
  value = module.media_s3.bucket_name
}

output "kong_internal_url" {
  description = "Cluster-internal Kong URL. Web app SSR routes use this as KONG_URL."
  value       = module.kong.internal_url
}

output "nameservers" {
  description = "Route53 nameservers. Add as 4 NS records at Namecheap for the 'twitter' subdomain. Required for ACM cert validation."
  value       = module.dns.nameservers
}

output "web_url" {
  value = module.ingress.web_url
}

output "api_url" {
  value = module.ingress.api_url
}

output "auth_url" {
  value = module.ingress.auth_url
}

output "alb_hostname" {
  value = module.ingress.alb_hostname
}

output "alarms_sns_topic_arn" {
  description = "SNS topic that fans out CloudWatch alarms. Confirm the email subscription after first apply."
  value       = module.observability.sns_topic_arn
}
