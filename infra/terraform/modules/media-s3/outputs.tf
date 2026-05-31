output "bucket_name" {
  description = "S3 bucket for user-uploaded media. Goes into media-service env as S3_BUCKET."
  value       = aws_s3_bucket.media.id
}

output "bucket_arn" {
  value = aws_s3_bucket.media.arn
}

output "public_url_base" {
  description = "Direct S3 URL prefix. Used by media-service to construct presigned URLs the browser fetches."
  value       = "https://${aws_s3_bucket.media.bucket_regional_domain_name}"
}

output "irsa_role_arn" {
  description = "IRSA role ARN. apps module annotates the media-service ServiceAccount with this."
  value       = module.media_irsa.iam_role_arn
}
