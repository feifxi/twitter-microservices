output "vpc_id" {
  value = module.vpc.vpc_id
}

output "vpc_cidr_block" {
  description = "Primary CIDR. Used to scope SG ingress rules to in-VPC traffic."
  value       = module.vpc.vpc_cidr_block
}

output "private_subnet_ids" {
  description = "Subnets for the data plane and EKS nodes."
  value       = module.vpc.private_subnets
}

output "public_subnet_ids" {
  description = "Subnets for internet-facing ALBs."
  value       = module.vpc.public_subnets
}

output "private_route_table_ids" {
  description = "Used when adding gateway VPC endpoints (DynamoDB, etc)."
  value       = module.vpc.private_route_table_ids
}

output "azs" {
  value = var.azs
}
