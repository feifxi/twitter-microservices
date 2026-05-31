variable "name_prefix" {
  description = "Bucket name will be <prefix>-media-<account-id>. Account ID suffix guarantees global uniqueness."
  type        = string
}

variable "oidc_provider_arn" {
  description = "From eks-cluster module. Used for the IRSA trust policy."
  type        = string
}

variable "apps_namespace" {
  description = "Namespace media-service runs in. The IRSA trust scopes to <namespace>:media-service."
  type        = string
}

variable "service_account_name" {
  type    = string
  default = "media-service"
}

variable "tags" {
  type    = map(string)
  default = {}
}
