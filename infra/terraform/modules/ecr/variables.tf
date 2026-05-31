variable "name_prefix" {
  description = "Optional repo name prefix. Empty by default so repo names are bare (user-service, web, ...) to match local Docker tags."
  type        = string
  default     = ""
}

variable "repository_names" {
  description = "ECR repos to create. Defaults to the 6 Go services + Next.js web + custom Keycloak image."
  type        = list(string)
  default = [
    "user-service",
    "tweet-service",
    "feed-service",
    "notification-service",
    "media-service",
    "search-service",
    "web",
    "keycloak",
  ]
}

variable "keep_last_n_tagged" {
  description = "Lifecycle: keep the most recent N tagged images per repo."
  type        = number
  default     = 10
}

variable "expire_untagged_after_days" {
  description = "Lifecycle: delete untagged images this many days after push."
  type        = number
  default     = 7
}

variable "tags" {
  type    = map(string)
  default = {}
}
