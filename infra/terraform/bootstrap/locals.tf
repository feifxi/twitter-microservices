data "aws_caller_identity" "current" {}
data "aws_partition" "current" {}

locals {
  tags = {
    Project    = var.project
    Env        = "bootstrap"
    ManagedBy  = "terraform"
    CostCenter = "personal"
  }
}
