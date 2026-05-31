output "alb_hostname" {
  description = "Raw ALB DNS name. Use as the CloudFront origin if you add a CDN later."
  value       = kubernetes_ingress_v1.main.status[0].load_balancer[0].ingress[0].hostname
}

output "alb_arn_suffix" {
  description = "ALB ARN suffix in the form app/<name>/<id>. The dimension CloudWatch uses for ALB metrics."
  value       = data.aws_lb.ingress.arn_suffix
}

output "web_url" {
  description = "Public HTTPS URL of the Next.js web app."
  value       = "https://${var.domain}"
}

output "api_url" {
  description = "Public HTTPS URL of Kong (API gateway)."
  value       = "https://api.${var.domain}"
}

output "auth_url" {
  description = "Public HTTPS URL of Keycloak."
  value       = "https://auth.${var.domain}"
}
