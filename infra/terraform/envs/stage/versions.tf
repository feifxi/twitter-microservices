terraform {
  required_version = "~> 1.9"
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.70"
    }
    kubernetes = {
      source = "hashicorp/kubernetes"
      # Pinned to 2.35.x deliberately. 2.36+ introduced "resource identity"
      # tracking that errors on resources created by older provider versions
      # with "Unexpected Identity Change". Stay on 2.35.x until the upstream
      # bug is fixed.
      version = "~> 2.35.0"
    }
    helm = {
      source  = "hashicorp/helm"
      version = "~> 3.0"
    }
    kubectl = {
      source  = "alekc/kubectl"
      version = "~> 2.1"
    }
  }
}
