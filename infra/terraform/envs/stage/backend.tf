terraform {
  backend "s3" {
    # bucket / region are passed at `terraform init` time via `make stage-init`,
    # which reads them from the bootstrap stack's outputs.
    # use_lockfile = S3-native conditional-write locking (TF 1.11+). Replaces the
    # older DynamoDB lock pattern, so no separate lock table needed.
    key          = "stage/terraform.tfstate"
    encrypt      = true
    use_lockfile = true
  }
}
