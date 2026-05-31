data "aws_region" "current" {}

locals {
  num_azs          = length(var.azs)
  private_subnets  = [for i in range(local.num_azs) : cidrsubnet(var.cidr, 4, i)]
  public_subnets   = [for i in range(local.num_azs) : cidrsubnet(var.cidr, 8, i + 100)]
  eks_cluster_name = var.eks_cluster_name != "" ? var.eks_cluster_name : var.name
}

module "vpc" {
  source  = "terraform-aws-modules/vpc/aws"
  version = "~> 5.13"

  name = var.name
  cidr = var.cidr

  azs             = var.azs
  private_subnets = local.private_subnets
  public_subnets  = local.public_subnets

  enable_nat_gateway   = true
  single_nat_gateway   = var.single_nat_gateway
  enable_dns_hostnames = true
  enable_dns_support   = true

  # ELB discovery: aws-load-balancer-controller reads these tags to decide
  # which subnets to put internet-facing vs internal ALBs in.
  public_subnet_tags = {
    "kubernetes.io/role/elb"                          = "1"
    "kubernetes.io/cluster/${local.eks_cluster_name}" = "shared"
  }
  private_subnet_tags = {
    "kubernetes.io/role/internal-elb"                 = "1"
    "kubernetes.io/cluster/${local.eks_cluster_name}" = "shared"
  }

  tags = var.tags
}

# S3 gateway endpoint is free and saves NAT egress whenever a pod talks to S3
# (media-service, MSK tiered storage, Aurora backups, ECR layer storage).
resource "aws_vpc_endpoint" "s3" {
  vpc_id            = module.vpc.vpc_id
  service_name      = "com.amazonaws.${data.aws_region.current.name}.s3"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = module.vpc.private_route_table_ids

  tags = merge(var.tags, { Name = "${var.name}-s3-endpoint" })
}
