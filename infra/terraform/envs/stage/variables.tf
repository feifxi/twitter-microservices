variable "project" {
  type    = string
  default = "twitter-mc"
}

variable "region" {
  type    = string
  default = "ap-southeast-1"
}

variable "image_tag" {
  description = "Container image tag deployed across every service. Phase 10 CI overrides via -var."
  type        = string
  default     = "latest"
}

variable "root_domain" {
  description = "Your registered domain (the thing at the registrar). Change this if you move the project to a new domain."
  type        = string
  default     = "chanombude.me"
}

variable "subdomain" {
  description = "Project subdomain under root_domain. Final app URL = <subdomain>.<root_domain>. Auto-derived: api.<subdomain>.<root_domain>, auth.<subdomain>.<root_domain>."
  type        = string
  default     = "twitter"
}

variable "alarm_email" {
  description = "Email address that receives CloudWatch alarm notifications. Confirm the SNS subscription email after first apply."
  type        = string
  default     = "you@example.com"
}
