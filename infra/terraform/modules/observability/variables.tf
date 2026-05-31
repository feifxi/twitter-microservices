variable "name" {
  description = "Prefix for IAM roles, SNS topic, and alarm names. Usually the cluster name."
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

variable "alarm_email" {
  description = "SNS topic subscriber. Confirm the subscription email after first apply or alarms go nowhere."
  type        = string
}

variable "msk_cluster_name" {
  description = "MSK cluster name. Dimension for the AWS/Kafka consumer lag alarm."
  type        = string
}

variable "alb_arn_suffix" {
  description = "ALB ARN suffix (app/<name>/<id>). Dimension for the AWS/ApplicationELB 5xx alarm."
  type        = string
}

variable "aurora_cluster_identifier" {
  description = "Aurora DB cluster identifier. Dimension for the AWS/RDS CPU alarm."
  type        = string
}

variable "tags" {
  type    = map(string)
  default = {}
}
