variable "name" {
  description = "Logical VPC name. Also used as the EKS cluster name unless overridden, so the subnet ELB-discovery tags line up."
  type        = string
}

variable "cidr" {
  description = "Primary VPC CIDR block. /16 gives ~65k IPs across 2 AZs (private /20, public /24)."
  type        = string
  default     = "10.0.0.0/16"
}

variable "azs" {
  description = "Availability zone names. The env stack picks the first N from aws_availability_zones."
  type        = list(string)
}

variable "single_nat_gateway" {
  description = "Use one NAT across all private subnets (cost-optimised). False would create one NAT per private subnet (terraform-aws-modules/vpc default). For one-per-AZ specifically, the upstream module also needs `one_nat_gateway_per_az = true` which this wrapper doesn't expose."
  type        = bool
  default     = true
}

variable "eks_cluster_name" {
  description = "Cluster name for the kubernetes.io/cluster/<name>=shared subnet tag. Defaults to var.name."
  type        = string
  default     = ""
}

variable "tags" {
  description = "Tags merged onto every resource the module creates."
  type        = map(string)
  default     = {}
}
