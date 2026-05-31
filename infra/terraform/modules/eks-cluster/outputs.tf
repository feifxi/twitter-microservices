output "cluster_name" {
  value = module.eks.cluster_name
}

output "cluster_endpoint" {
  value = module.eks.cluster_endpoint
}

output "cluster_certificate_authority_data" {
  description = "Base64 cluster CA bundle. Pair with cluster_endpoint to configure the kubernetes provider."
  value       = module.eks.cluster_certificate_authority_data
}

output "oidc_provider_arn" {
  description = "IRSA trust-policy federated identifier. Consumed by every IRSA role in the cluster."
  value       = module.eks.oidc_provider_arn
}

output "oidc_provider_url" {
  description = "Without the https:// prefix; used in trust-policy sub/aud conditions."
  value       = module.eks.cluster_oidc_issuer_url
}

output "cluster_security_group_id" {
  description = "Control-plane SG. Data-plane SGs eventually allow ingress from this + node SG."
  value       = module.eks.cluster_security_group_id
}

output "node_security_group_id" {
  description = "Node SG. Data-plane SGs add ingress from this to replace VPC-CIDR-wide rules."
  value       = module.eks.node_security_group_id
}
