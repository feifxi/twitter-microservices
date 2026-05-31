variable "name" {
  description = "Logical name. Prefix for the cluster, subnet group, and security group."
  type        = string
}

variable "vpc_id" {
  type = string
}

variable "vpc_cidr_block" {
  description = "Ingress source for the cluster SG. Narrow to EKS-node SG later for tighter isolation."
  type        = string
}

variable "private_subnet_ids" {
  type = list(string)
}

variable "database_name" {
  description = "Initial database. Services share this DB with schema-per-service (search_path=users, search_path=tweet, …)."
  type        = string
  default     = "twitter"
}

variable "engine_version" {
  description = "Aurora PostgreSQL engine version. Aurora supports a subset of Postgres versions — check aws_rds_engine_versions if applying fails."
  type        = string
  default     = "16.4"
}

variable "min_capacity" {
  description = "Serverless v2 min ACU. 0 enables auto-pause after seconds_until_auto_pause."
  type        = number
  default     = 0
}

variable "max_capacity" {
  description = "Serverless v2 max ACU."
  type        = number
  default     = 2
}

variable "seconds_until_auto_pause" {
  description = "Idle time before Aurora SV2 scales to 0. Cold-start back to min_capacity ~15s."
  type        = number
  default     = 3600
}

variable "tags" {
  type    = map(string)
  default = {}
}
