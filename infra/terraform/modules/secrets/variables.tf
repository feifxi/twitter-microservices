variable "name_prefix" {
  description = "Prefix for Secrets Manager secret names. Becomes the AWS path (e.g. twitter-mc-stage/openai-api-key)."
  type        = string
}

variable "region" {
  description = "AWS region for the SecretStore provider config."
  type        = string
}

variable "aurora_master_secret_arn" {
  description = "ARN of the RDS-managed Aurora password secret (from the data-aurora module). The ExternalSecret reads .username and .password from it."
  type        = string
}

variable "apps_namespace" {
  description = "Namespace where application workloads deploy. Created here so ExternalSecrets target the same namespace."
  type        = string
  default     = "apps"
}

variable "recovery_window_days" {
  description = "Soft-delete window. 0 = immediate hard delete on destroy, so the next `make stage-up` can reuse the same secret names. 7-30 would be the right choice for prod."
  type        = number
  default     = 0
}

variable "tags" {
  type    = map(string)
  default = {}
}
