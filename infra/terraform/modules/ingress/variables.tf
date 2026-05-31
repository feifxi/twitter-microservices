variable "namespace" {
  description = "Namespace where the Ingress lives (same as Kong/Keycloak/web services)."
  type        = string
}

variable "domain" {
  description = "Apex of the zone (e.g. twitter.chanombude.dev). Subdomains are derived: api.<domain> -> kong, auth.<domain> -> keycloak, <domain> -> web."
  type        = string
}

variable "zone_id" {
  description = "Route53 zone id where A-ALIAS records are created."
  type        = string
}

variable "certificate_arn" {
  description = "Issued ACM cert ARN from the cert module. Must cover <domain> and *.<domain>."
  type        = string
}

variable "web_service_name" {
  type    = string
  default = "web"
}

variable "web_service_port" {
  type    = number
  default = 3000
}

variable "kong_service_name" {
  type    = string
  default = "kong"
}

variable "kong_service_port" {
  type    = number
  default = 8000
}

variable "keycloak_service_name" {
  type    = string
  default = "keycloak"
}

variable "keycloak_service_port" {
  type    = number
  default = 8080
}

variable "tags" {
  type    = map(string)
  default = {}
}
