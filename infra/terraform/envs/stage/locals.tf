locals {
  env    = "stage"
  domain = "${var.subdomain}.${var.root_domain}"
  tags = {
    Project    = var.project
    Env        = local.env
    ManagedBy  = "terraform"
    CostCenter = "personal"
  }
}
