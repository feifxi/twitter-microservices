data "aws_caller_identity" "current" {}

resource "aws_security_group" "this" {
  name        = "${var.name}-opensearch"
  description = "OpenSearch - ingress from in-VPC on 443 (HTTPS)"
  vpc_id      = var.vpc_id
  tags        = var.tags
}

resource "aws_vpc_security_group_ingress_rule" "https_in" {
  security_group_id = aws_security_group.this.id
  cidr_ipv4         = var.vpc_cidr_block
  from_port         = 443
  to_port           = 443
  ip_protocol       = "tcp"
}

resource "aws_opensearch_domain" "this" {
  domain_name    = "${var.name}-os"
  engine_version = var.engine_version

  cluster_config {
    instance_type            = var.instance_type
    instance_count           = var.instance_count
    dedicated_master_enabled = false
    zone_awareness_enabled   = false
  }

  ebs_options {
    ebs_enabled = true
    volume_size = var.ebs_volume_size_gb
    volume_type = "gp3"
  }

  vpc_options {
    subnet_ids         = slice(var.private_subnet_ids, 0, var.instance_count)
    security_group_ids = [aws_security_group.this.id]
  }

  domain_endpoint_options {
    enforce_https       = true
    tls_security_policy = "Policy-Min-TLS-1-2-2019-07"
  }

  encrypt_at_rest {
    enabled = true
  }

  node_to_node_encryption {
    enabled = true
  }

  # Open inside the VPC: any AWS principal can call. Real isolation is the SG.
  access_policies = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { AWS = "*" }
      Action    = "es:*"
      Resource  = "arn:aws:es:*:${data.aws_caller_identity.current.account_id}:domain/${var.name}-os/*"
    }]
  })

  tags = var.tags
}
