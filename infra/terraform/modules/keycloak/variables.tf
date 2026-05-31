variable "namespace" {
  description = "Kubernetes namespace to deploy into. Reuses the apps namespace from the secrets module so existing ExternalSecrets are mountable."
  type        = string
}

variable "image" {
  description = "ECR repo URL for the custom Keycloak image (with SPI baked in)."
  type        = string
}

variable "image_tag" {
  description = "Image tag. CI overrides with git short SHA; default 'latest' works for stage."
  type        = string
  default     = "latest"
}

variable "hostname" {
  description = "Public hostname Keycloak accepts requests for (others are rejected). e.g. auth.twitter.chanombude.me. The Ingress routes this host to the keycloak Service."
  type        = string
}

variable "web_domain" {
  description = "Web app's public domain (e.g. twitter.chanombude.me). Used for the twitter-app client's redirect URIs."
  type        = string
}

variable "aurora_endpoint" {
  description = "Aurora writer endpoint (host:port). Keycloak uses JDBC postgres URL."
  type        = string
}

variable "aurora_database" {
  description = "Database name on the Aurora cluster. Keycloak creates its tables in the public schema."
  type        = string
  default     = "twitter"
}

variable "aurora_credentials_secret" {
  description = "Name of the k8s Secret holding DATABASE_USERNAME and DATABASE_PASSWORD (synced by ESO from RDS-managed secret)."
  type        = string
  default     = "aurora-credentials"
}

variable "app_secrets_secret" {
  description = "Name of the k8s Secret holding KEYCLOAK_ADMIN_PASSWORD (synced by ESO from AWS Secrets Manager)."
  type        = string
  default     = "app-secrets"
}

variable "replicas" {
  type    = number
  default = 1
}

variable "resources" {
  type = object({
    requests = object({ cpu = string, memory = string })
    limits   = object({ cpu = string, memory = string })
  })
  default = {
    requests = { cpu = "100m", memory = "512Mi" }
    limits   = { cpu = "1000m", memory = "1Gi" }
  }
}

variable "tags" {
  description = "Tags merged into pod labels (k8s labels, not AWS tags)."
  type        = map(string)
  default     = {}
}
