data "aws_caller_identity" "current" {}

resource "aws_s3_bucket" "media" {
  bucket = "${var.name_prefix}-media-${data.aws_caller_identity.current.account_id}"

  # Stage is recreated on demand; empty bucket without manual cleanup on destroy.
  force_destroy = true

  tags = var.tags
}

resource "aws_s3_bucket_public_access_block" "media" {
  bucket                  = aws_s3_bucket.media.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

# Browser fetches uploaded media via presigned URLs, so CORS only needs to allow
# the web app origin to read.
resource "aws_s3_bucket_cors_configuration" "media" {
  bucket = aws_s3_bucket.media.id

  cors_rule {
    allowed_methods = ["GET", "HEAD"]
    allowed_origins = ["*"]
    allowed_headers = ["*"]
    max_age_seconds = 3600
  }
}

resource "aws_s3_bucket_server_side_encryption_configuration" "media" {
  bucket = aws_s3_bucket.media.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

# IRSA role for media-service. Trust policy scoped to the specific SA.
module "media_irsa" {
  source  = "terraform-aws-modules/iam/aws//modules/iam-role-for-service-accounts-eks"
  version = "~> 5.50"

  role_name = "${var.name_prefix}-media-service"

  role_policy_arns = {
    s3 = aws_iam_policy.media_s3.arn
  }

  oidc_providers = {
    main = {
      provider_arn               = var.oidc_provider_arn
      namespace_service_accounts = ["${var.apps_namespace}:${var.service_account_name}"]
    }
  }

  tags = var.tags
}

resource "aws_iam_policy" "media_s3" {
  name        = "${var.name_prefix}-media-s3"
  description = "media-service: presigned URL generation + read/write to the media bucket only"

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect = "Allow"
      Action = [
        "s3:PutObject",
        "s3:GetObject",
        "s3:DeleteObject",
        "s3:AbortMultipartUpload",
        "s3:ListMultipartUploadParts",
      ]
      Resource = "${aws_s3_bucket.media.arn}/*"
      }, {
      Effect   = "Allow"
      Action   = ["s3:ListBucket", "s3:GetBucketLocation"]
      Resource = aws_s3_bucket.media.arn
    }]
  })
}
