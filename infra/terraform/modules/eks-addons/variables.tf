variable "name" {
  description = "Prefix for IAM role names. Usually the cluster name."
  type        = string
}

variable "cluster_name" {
  description = "Passed to the ALB controller chart so it tags ALBs with the cluster name."
  type        = string
}

variable "oidc_provider_arn" {
  description = "From eks-cluster module. Used by IRSA trust policies."
  type        = string
}

variable "alb_controller_chart_version" {
  type    = string
  default = "1.10.0"
}

variable "external_secrets_chart_version" {
  type    = string
  default = "0.10.7"
}

variable "metrics_server_chart_version" {
  type    = string
  default = "3.12.2"
}

variable "tags" {
  type    = map(string)
  default = {}
}
