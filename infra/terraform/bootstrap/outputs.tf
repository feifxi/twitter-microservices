output "state_bucket" {
  description = "S3 bucket holding remote terraform state for envs/*. Wired into envs/stage via make stage-init."
  value       = aws_s3_bucket.tfstate.bucket
}

output "lock_table" {
  description = "DynamoDB table used for terraform state locking."
  value       = aws_dynamodb_table.tflock.name
}

output "region" {
  description = "AWS region the state lives in."
  value       = var.region
}

output "github_actions_role_arn" {
  description = "Role-to-assume for GitHub Actions OIDC. Consumed by CI workflows."
  value       = aws_iam_role.github_actions.arn
}

output "alarms_topic_arn" {
  description = "SNS topic for CloudWatch alarms."
  value       = aws_sns_topic.alarms.arn
}
