variable "name" {
  type = string
}

variable "vpc_id" {
  type = string
}

variable "vpc_cidr_block" {
  type = string
}

variable "private_subnet_ids" {
  description = "instance_count must equal len(subnet_ids) when zone_awareness is on. Single-node uses one subnet."
  type        = list(string)
}

variable "instance_type" {
  type    = string
  default = "t3.small.search"
}

variable "instance_count" {
  type    = number
  default = 1
}

variable "engine_version" {
  description = "AWS-managed OpenSearch tracks behind upstream. Local docker is 3.5 — verify search-service queries still work if you change this."
  type        = string
  default     = "OpenSearch_2.17"
}

variable "ebs_volume_size_gb" {
  type    = number
  default = 20
}

variable "tags" {
  type    = map(string)
  default = {}
}
