#!/bin/bash
awslocal s3 mb s3://twitter-media --region ap-southeast-1
awslocal s3api put-bucket-cors --bucket twitter-media --cors-configuration '{
  "CORSRules": [{
    "AllowedOrigins": ["*"],
    "AllowedMethods": ["GET", "PUT", "POST"],
    "AllowedHeaders": ["*"]
  }]
}'
echo "→ S3 bucket twitter-media ready"
