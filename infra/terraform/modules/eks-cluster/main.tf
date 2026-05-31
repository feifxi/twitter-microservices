module "eks" {
  source  = "terraform-aws-modules/eks/aws"
  version = "~> 20.31"

  cluster_name    = var.name
  cluster_version = var.cluster_version

  vpc_id     = var.vpc_id
  subnet_ids = var.private_subnet_ids

  # Public endpoint stays on so kubectl from a laptop works without a bastion.
  # Tighten with cluster_endpoint_public_access_cidrs if exposure becomes a concern.
  cluster_endpoint_public_access  = true
  cluster_endpoint_private_access = true

  # Access-entries replace the legacy aws-auth ConfigMap. Caller (whoever runs
  # terraform apply) is granted cluster admin via access entry automatically.
  enable_cluster_creator_admin_permissions = true

  # v20+ of this module removed default addons. Without explicit cluster_addons,
  # vpc-cni isn't installed -> nodes have no pod networking -> kubelet never goes
  # Ready -> "NodeCreationFailure: Instances failed to join the kubernetes cluster".
  # before_compute = true on vpc-cni installs CNI before node group creation so
  # there's no race window where nodes boot before networking is ready.
  cluster_addons = {
    coredns    = { most_recent = true }
    kube-proxy = { most_recent = true }
    vpc-cni = {
      most_recent    = true
      before_compute = true
    }
  }

  eks_managed_node_groups = {
    main = {
      ami_type       = "AL2023_x86_64_STANDARD"
      instance_types = var.node_instance_types
      capacity_type  = "ON_DEMAND"

      min_size     = var.node_min_size
      max_size     = var.node_max_size
      desired_size = var.node_desired_size
    }
  }

  tags = var.tags
}
