provider "aws" {
  region = var.region
  default_tags {
    tags = local.tags
  }
}

# `exec` auth: defers token fetch to apply time, so the providers can reference
# module.eks_cluster outputs that don't exist until the cluster is created.
# Without exec, the providers would try to authenticate at plan time and fail.
provider "kubernetes" {
  host                   = module.eks_cluster.cluster_endpoint
  cluster_ca_certificate = base64decode(module.eks_cluster.cluster_certificate_authority_data)

  exec {
    api_version = "client.authentication.k8s.io/v1beta1"
    command     = "aws"
    args = [
      "eks", "get-token",
      "--cluster-name", module.eks_cluster.cluster_name,
      "--region", var.region,
    ]
  }
}

provider "helm" {
  kubernetes = {
    host                   = module.eks_cluster.cluster_endpoint
    cluster_ca_certificate = base64decode(module.eks_cluster.cluster_certificate_authority_data)

    exec = {
      api_version = "client.authentication.k8s.io/v1beta1"
      command     = "aws"
      args = [
        "eks", "get-token",
        "--cluster-name", module.eks_cluster.cluster_name,
        "--region", var.region,
      ]
    }
  }
}

provider "kubectl" {
  # alekc/kubectl eagerly evaluates its config. When module.eks_cluster outputs
  # are null (during teardown after the cluster's been destroyed), passing those
  # as host/cert errors with "no configuration provided". load_config_file=true
  # lets the provider fall back to ~/.kube/config so it can configure itself
  # even when the cluster outputs are gone; resources still target the actual
  # cluster (via the same kubeconfig) during apply.
  load_config_file = true
  config_context   = "twitter-mc-stage"
}
