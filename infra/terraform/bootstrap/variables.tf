variable "project" {
  description = "Project name; prefix for all resources."
  type        = string
  default     = "twitter-mc"
}

variable "region" {
  description = "AWS region. The state bucket lives here."
  type        = string
  default     = "ap-southeast-1"
}

variable "budget_email" {
  description = "Email subscribed to the AWS Budgets alarms and the CloudWatch SNS topic."
  type        = string
}

variable "monthly_budget_usd" {
  description = "Monthly spend cap. Triggers 80% forecast + 100% actual notifications."
  type        = number
  default     = 50
}

variable "github_repo" {
  description = "owner/name of the GitHub repo allowed to assume the OIDC role (Phase 10 CI)."
  type        = string
  default     = "feifxi/twitter-microservices"
}
