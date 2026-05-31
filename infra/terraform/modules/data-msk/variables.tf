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
  description = "Must have length >= broker_count, and broker_count must be a multiple of the subnets used."
  type        = list(string)
}

variable "broker_count" {
  description = "Number of broker nodes. MSK requires this to be a multiple of client_subnets count, AND client_subnets must be 2 or 3. So minimum is 2. Production reference is 3 (1/AZ)."
  type        = number
  default     = 2
}

variable "instance_type" {
  type    = string
  default = "kafka.t3.small"
}

variable "kafka_version" {
  type    = string
  default = "3.6.0"
}

variable "ebs_volume_size_gb" {
  type    = number
  default = 50
}

variable "tags" {
  type    = map(string)
  default = {}
}
