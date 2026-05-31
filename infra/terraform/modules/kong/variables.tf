variable "namespace" {
  type = string
}

variable "realm_public_key_pem" {
  description = "RSA public key (PEM) from the keycloak module. Kong validates JWT signatures against this without round-tripping to JWKS."
  type        = string
  sensitive   = false
}

variable "realm_issuer" {
  description = "JWT 'iss' claim that Kong matches to the consumer credential. Same as Keycloak's realm public URL."
  type        = string
}

variable "image" {
  description = "Kong image. Use the official OSS image (library/kong on Docker Hub); enterprise not needed for stage. No -alpine variants exist anymore — Kong dropped them post-3.5."
  type        = string
  default     = "kong:3.9"
}

variable "replicas" {
  type    = number
  default = 1
}

# Each entry registers a service + path in Kong's DB-less config.
# All routes get the jwt + pre-function plugins from main.tf (configured at
# Kong load-time, not per route here).
variable "routes" {
  description = "Map of upstream-service-name -> { port, paths }. Keys must match k8s Service names in the same namespace."
  type = map(object({
    port  = number
    paths = list(string)
  }))
  default = {
    "user-service"         = { port = 8001, paths = ["/v1/users", "/v1/profiles", "/v1/follows"] }
    "tweet-service"        = { port = 8002, paths = ["/v1/tweets", "/v1/retweets", "/v1/likes", "/v1/replies"] }
    "feed-service"         = { port = 8003, paths = ["/v1/feeds", "/v1/trending"] }
    "notification-service" = { port = 8004, paths = ["/v1/notifications"] }
    "media-service"        = { port = 8005, paths = ["/v1/media"] }
    "search-service"       = { port = 8006, paths = ["/v1/search"] }
  }
  # Note: /internal/users intentionally omitted. The Keycloak SPI calls
  # user-service directly cluster-internal (not via Kong), so it doesn't need
  # a route here and isn't exposed publicly.
}

variable "tags" {
  type    = map(string)
  default = {}
}
