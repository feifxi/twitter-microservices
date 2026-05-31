terraform {
  required_version = "~> 1.9"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.70"
    }
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.35.0"
    }
    # alekc/kubectl manifest resource doesn't dry-run against the cluster at
    # plan time. hashicorp/kubernetes kubernetes_manifest does, which breaks
    # first-apply from clean state because the cluster doesn't exist yet.
    kubectl = {
      source  = "alekc/kubectl"
      version = "~> 2.1"
    }
  }
}
