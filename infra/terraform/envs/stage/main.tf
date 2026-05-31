data "aws_availability_zones" "available" {
  state = "available"
}

locals {
  resource_prefix = "${var.project}-${local.env}"
}


module "vpc" {
  source = "../../modules/vpc"

  name = local.resource_prefix
  azs  = slice(data.aws_availability_zones.available.names, 0, 2)
  tags = local.tags
}

# data plane
module "aurora" {
  source = "../../modules/data-aurora"

  name               = local.resource_prefix
  vpc_id             = module.vpc.vpc_id
  vpc_cidr_block     = module.vpc.vpc_cidr_block
  private_subnet_ids = module.vpc.private_subnet_ids
  tags               = local.tags
}

module "elasticache" {
  source = "../../modules/data-elasticache"

  name               = local.resource_prefix
  vpc_id             = module.vpc.vpc_id
  vpc_cidr_block     = module.vpc.vpc_cidr_block
  private_subnet_ids = module.vpc.private_subnet_ids
  tags               = local.tags
}

module "msk" {
  source = "../../modules/data-msk"

  name               = local.resource_prefix
  vpc_id             = module.vpc.vpc_id
  vpc_cidr_block     = module.vpc.vpc_cidr_block
  private_subnet_ids = module.vpc.private_subnet_ids
  tags               = local.tags
}

module "opensearch" {
  source = "../../modules/data-opensearch"

  name               = local.resource_prefix
  vpc_id             = module.vpc.vpc_id
  vpc_cidr_block     = module.vpc.vpc_cidr_block
  private_subnet_ids = module.vpc.private_subnet_ids
  tags               = local.tags
}

# EKS cluster + in-cluster addons
module "eks_cluster" {
  source = "../../modules/eks-cluster"

  name               = local.resource_prefix
  vpc_id             = module.vpc.vpc_id
  private_subnet_ids = module.vpc.private_subnet_ids
  tags               = local.tags
}

module "eks_addons" {
  source = "../../modules/eks-addons"

  name              = local.resource_prefix
  cluster_name      = module.eks_cluster.cluster_name
  oidc_provider_arn = module.eks_cluster.oidc_provider_arn
  tags              = local.tags

  # Wait for the entire eks_cluster module (cluster + node group) to be applied
  # before starting addons. Without this, EBS CSI addon may start while the node
  # group is still being created -> CSI DaemonSet has nowhere to schedule ->
  # addon stuck in DEGRADED for 20 min until timeout.
  depends_on = [module.eks_cluster]
}

# ECR repos
module "ecr" {
  source = "../../modules/ecr"

  tags = local.tags
}

# Secrets Manager + ESO wiring
module "secrets" {
  source = "../../modules/secrets"

  name_prefix              = local.resource_prefix
  region                   = var.region
  aurora_master_secret_arn = module.aurora.master_user_secret_arn
  tags                     = local.tags

  # ExternalSecret + ClusterSecretStore CRDs come from the ESO helm release.
  depends_on = [module.eks_addons]
}

# Keycloak (custom image with SPI, points at Aurora)
module "keycloak" {
  source = "../../modules/keycloak"

  namespace       = module.secrets.apps_namespace
  image           = module.ecr.repository_urls["keycloak"]
  image_tag       = var.image_tag
  hostname        = "auth.${local.domain}"
  web_domain      = local.domain
  aurora_endpoint = module.aurora.cluster_endpoint
  tags            = local.tags

  depends_on = [module.secrets]
}

# media S3 bucket + IRSA for media-service
module "media_s3" {
  source = "../../modules/media-s3"

  name_prefix       = local.resource_prefix
  oidc_provider_arn = module.eks_cluster.oidc_provider_arn
  apps_namespace    = module.secrets.apps_namespace
  tags              = local.tags
}

# app services + web (6 Go services + Next.js)
module "apps" {
  source = "../../modules/apps"

  namespace                = module.secrets.apps_namespace
  image_tag                = var.image_tag
  ecr_urls                 = module.ecr.repository_urls
  aurora_endpoint          = module.aurora.cluster_endpoint
  aurora_master_secret_arn = module.aurora.master_user_secret_arn
  redis_endpoint           = module.elasticache.endpoint
  kafka_brokers            = module.msk.bootstrap_brokers
  opensearch_endpoint      = module.opensearch.endpoint
  keycloak_internal_url    = module.keycloak.internal_url
  domain                   = local.domain
  media_s3_bucket          = module.media_s3.bucket_name
  media_s3_public_url_base = module.media_s3.public_url_base
  media_irsa_role_arn      = module.media_s3.irsa_role_arn
  tags                     = local.tags

  depends_on = [module.secrets, module.keycloak]
}

# Kong API gateway: path routing + JWT validation (RS256 against the realm's
# public key) + Lua pre-function that injects X-User-* headers from JWT claims.
module "kong" {
  source = "../../modules/kong"

  namespace            = module.secrets.apps_namespace
  realm_public_key_pem = module.keycloak.realm_public_key_pem
  realm_issuer         = module.keycloak.realm_issuer
  tags                 = local.tags

  depends_on = [module.apps]
}

# Route53 zone for the subdomain. Prints NS records to apply output.
module "dns" {
  source = "../../modules/dns"

  domain = local.domain
  tags   = local.tags
}

# ACM cert (regional). Blocks apply until cert is ISSUED. If NS records
# aren't at the registrar yet, terraform pauses here until you add them.
module "cert" {
  source = "../../modules/cert"

  domain  = local.domain
  zone_id = module.dns.zone_id
  tags    = local.tags
}

# Single ALB Ingress with host-based routing to web / kong / keycloak.
# Creates the A-ALIAS records in the Route53 zone pointing at the ALB.
module "ingress" {
  source = "../../modules/ingress"

  namespace             = module.secrets.apps_namespace
  domain                = local.domain
  zone_id               = module.dns.zone_id
  certificate_arn       = module.cert.certificate_arn
  web_service_name      = module.apps.web_service_name
  kong_service_name     = module.kong.service_name
  keycloak_service_name = module.keycloak.service_name
  tags                  = local.tags

  depends_on = [module.apps, module.kong, module.keycloak]
}

# HPA on tweet-service + feed-service (CPU 60%, 1->5 replicas). Requires
# metrics-server (installed by eks_addons).
module "autoscaling" {
  source = "../../modules/autoscaling"

  namespace = module.secrets.apps_namespace

  depends_on = [module.apps, module.eks_addons]
}

# Container Insights (metrics + logs) via the managed amazon-cloudwatch-observability
# addon + 3 CloudWatch alarms wired to an SNS email subscription.
module "observability" {
  source = "../../modules/observability"

  name              = local.resource_prefix
  cluster_name      = module.eks_cluster.cluster_name
  oidc_provider_arn = module.eks_cluster.oidc_provider_arn
  region            = var.region
  alarm_email       = var.alarm_email
  msk_cluster_name  = module.msk.cluster_name
  alb_arn_suffix    = module.ingress.alb_arn_suffix
  apps_namespace    = module.secrets.apps_namespace
  tags              = local.tags

  depends_on = [module.eks_addons, module.ingress, module.msk]
}

