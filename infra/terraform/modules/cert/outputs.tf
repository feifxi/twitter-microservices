output "certificate_arn" {
  description = "Issued cert ARN. ingress module passes this to the alb.ingress.kubernetes.io/certificate-arn annotation."
  value       = aws_acm_certificate_validation.this.certificate_arn
}
