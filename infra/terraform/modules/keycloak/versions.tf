terraform {
  required_version = "~> 1.9"
  required_providers {
    kubernetes = {
      source  = "hashicorp/kubernetes"
      version = "~> 2.35.0"
    }
    # Generates the RSA key pair baked into the realm. Same key is exported and
    # used by Kong's JWT plugin to validate tokens without round-tripping to JWKS.
    tls = {
      source  = "hashicorp/tls"
      version = "~> 4.0"
    }
  }
}
