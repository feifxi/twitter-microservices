variable "name" {
  description = "Prefix for IAM roles and SNS topic. Usually the cluster name."
  type        = string
}

variable "cluster_name" {
  description = "EKS cluster name. The CloudWatch Observability addon attaches to this."
  type        = string
}

variable "oidc_provider_arn" {
  description = "From eks-cluster module. Used by the IRSA trust policy."
  type        = string
}

variable "region" {
  type = string
}

variable "alarm_email" {
  description = "SNS topic subscriber. Confirm the subscription email after first apply or alarms go nowhere."
  type        = string
}

variable "msk_cluster_name" {
  description = "MSK cluster name. Used as the dimension for the Kafka consumer lag alarm."
  type        = string
}

variable "alb_arn_suffix" {
  description = "ALB ARN suffix (app/<name>/<id>). CloudWatch dimension for ALB 5xx alarm."
  type        = string
}

variable "apps_namespace" {
  description = "Namespace the Prometheus scrape config selects pods from."
  type        = string
  default     = "apps"
}

variable "tags" {
  type    = map(string)
  default = {}
}
