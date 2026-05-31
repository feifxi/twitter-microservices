variable "domain" {
  description = "Apex of the zone (e.g. twitter.chanombude.dev). The cert is issued for this + *.<domain>."
  type        = string
}

variable "zone_id" {
  description = "Route53 zone id where DNS-01 validation records are created."
  type        = string
}

variable "tags" {
  type    = map(string)
  default = {}
}
