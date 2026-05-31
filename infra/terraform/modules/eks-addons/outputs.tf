output "alb_controller_role_arn" {
  value = module.alb_controller_irsa.iam_role_arn
}

output "external_secrets_role_arn" {
  description = "IRSA role for the external-secrets ServiceAccount. Grants Secrets Manager read."
  value       = module.external_secrets_irsa.iam_role_arn
}

output "ebs_csi_role_arn" {
  value = module.ebs_csi_irsa.iam_role_arn
}
