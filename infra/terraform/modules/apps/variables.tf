variable "namespace" {
  type = string
}

variable "aurora_master_secret_arn" {
  description = "ARN of the RDS-managed Secrets Manager secret holding {username,password}. The aurora-urls ExternalSecret extracts from this and templates per-service DATABASE_URL."
  type        = string
}

variable "image_tag" {
  type    = string
  default = "latest"
}

variable "ecr_urls" {
  description = "Map of service short-name -> ECR repo URL (host/repo). From the ecr module."
  type        = map(string)
}

variable "aurora_endpoint" {
  description = "Aurora writer endpoint (host[:port])."
  type        = string
}

variable "aurora_database" {
  type    = string
  default = "twitter"
}

variable "aurora_credentials_secret" {
  description = "Existing k8s Secret (ESO-synced) with DATABASE_USERNAME / DATABASE_PASSWORD."
  type        = string
  default     = "aurora-credentials"
}

variable "app_secrets_secret" {
  description = "Existing k8s Secret (ESO-synced) with OPENAI_API_KEY, SERVICE_TOKEN, KEYCLOAK_ADMIN_*"
  type        = string
  default     = "app-secrets"
}

variable "redis_endpoint" {
  description = "host:port"
  type        = string
}

variable "kafka_brokers" {
  description = "Comma-separated bootstrap brokers."
  type        = string
}

variable "opensearch_endpoint" {
  description = "Full https URL."
  type        = string
}

variable "keycloak_internal_url" {
  description = "Cluster-internal Keycloak URL. Services hit this for admin API."
  type        = string
}

variable "domain" {
  description = "Public domain (e.g. twitter.chanombude.dev). Used to build APP_URL / KEYCLOAK_URL for the web."
  type        = string
}

variable "media_s3_bucket" {
  description = "From media-s3 module."
  type        = string
}

variable "media_s3_public_url_base" {
  description = "From media-s3 module."
  type        = string
}

variable "media_irsa_role_arn" {
  description = "From media-s3 module. Annotated onto the media-service ServiceAccount."
  type        = string
}

variable "tags" {
  type    = map(string)
  default = {}
}
