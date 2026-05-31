terraform {
  required_version = "~> 1.9"
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.35.0"
    }
    # ExternalSecret CRD instances use kubectl_manifest so terraform doesn't
    # try to dry-run them against the cluster at plan time.
    kubectl = {
      source  = "alekc/kubectl"
      version = "~> 2.1"
    }
  }
}
