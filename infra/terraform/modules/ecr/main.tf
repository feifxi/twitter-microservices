locals {
  full_names = { for n in var.repository_names : n => "${var.name_prefix}${n}" }
}

resource "aws_ecr_repository" "this" {
  for_each = local.full_names

  name                 = each.value
  image_tag_mutability = "MUTABLE"

  # Stage env is recreated on demand. Without force_delete, `make stage-down`
  # fails on any repo that has pushed images.
  force_delete = true

  image_scanning_configuration {
    scan_on_push = true
  }

  encryption_configuration {
    encryption_type = "AES256"
  }

  tags = var.tags
}

resource "aws_ecr_lifecycle_policy" "this" {
  for_each = aws_ecr_repository.this

  repository = each.value.name
  policy = jsonencode({
    rules = [
      {
        rulePriority = 1
        description  = "Expire untagged images after ${var.expire_untagged_after_days} days"
        selection = {
          tagStatus   = "untagged"
          countType   = "sinceImagePushed"
          countUnit   = "days"
          countNumber = var.expire_untagged_after_days
        }
        action = { type = "expire" }
      },
      {
        rulePriority = 2
        description  = "Keep the last ${var.keep_last_n_tagged} tagged images"
        selection = {
          tagStatus   = "any"
          countType   = "imageCountMoreThan"
          countNumber = var.keep_last_n_tagged
        }
        action = { type = "expire" }
      },
    ]
  })
}
