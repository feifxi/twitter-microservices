variable "domain" {
  description = "Subdomain dedicated to this stage (e.g. twitter.chanombude.dev). A Route53 hosted zone is created for it; you delegate at the registrar by adding NS records."
  type        = string
}

variable "tags" {
  type    = map(string)
  default = {}
}
