# ALB controller: needs broad EC2/ELB rights. The IRSA submodule bakes the
# kubernetes-sigs/aws-load-balancer-controller policy so we don't hand-maintain ~150 lines of JSON.
module "alb_controller_irsa" {
  source  = "terraform-aws-modules/iam/aws//modules/iam-role-for-service-accounts-eks"
  version = "~> 5.50"

  role_name                              = "${var.name}-alb-controller"
  attach_load_balancer_controller_policy = true

  oidc_providers = {
    main = {
      provider_arn               = var.oidc_provider_arn
      namespace_service_accounts = ["kube-system:aws-load-balancer-controller"]
    }
  }

  tags = var.tags
}

# External Secrets Operator: secretsmanager:GetSecretValue + DescribeSecret scoped
# to all secrets in the account. Tighten to specific ARNs once secrets are stable.
module "external_secrets_irsa" {
  source  = "terraform-aws-modules/iam/aws//modules/iam-role-for-service-accounts-eks"
  version = "~> 5.50"

  role_name                      = "${var.name}-external-secrets"
  attach_external_secrets_policy = true

  oidc_providers = {
    main = {
      provider_arn               = var.oidc_provider_arn
      namespace_service_accounts = ["external-secrets:external-secrets"]
    }
  }

  tags = var.tags
}

# EBS CSI driver: lets PVCs back onto EBS volumes. Needed for any chart that
# wants persistent storage (Keycloak realm cache, future Prometheus, etc).
# Installed as an EKS-managed addon so AWS handles version upgrades.
module "ebs_csi_irsa" {
  source  = "terraform-aws-modules/iam/aws//modules/iam-role-for-service-accounts-eks"
  version = "~> 5.50"

  role_name             = "${var.name}-ebs-csi"
  attach_ebs_csi_policy = true

  oidc_providers = {
    main = {
      provider_arn               = var.oidc_provider_arn
      namespace_service_accounts = ["kube-system:ebs-csi-controller-sa"]
    }
  }

  tags = var.tags
}

resource "aws_eks_addon" "ebs_csi" {
  cluster_name                = var.cluster_name
  addon_name                  = "aws-ebs-csi-driver"
  service_account_role_arn    = module.ebs_csi_irsa.iam_role_arn
  resolve_conflicts_on_create = "OVERWRITE"
  resolve_conflicts_on_update = "OVERWRITE"
  tags                        = var.tags
}

resource "helm_release" "alb_controller" {
  name       = "aws-load-balancer-controller"
  repository = "https://aws.github.io/eks-charts"
  chart      = "aws-load-balancer-controller"
  version    = var.alb_controller_chart_version
  namespace  = "kube-system"

  # On failed install, helm rolls back + deletes partial state so the next
  # `terraform apply` doesn't hit "release name already in use".
  atomic = true

  set = [
    {
      name  = "clusterName"
      value = var.cluster_name
    },
    {
      name  = "serviceAccount.create"
      value = "true"
    },
    {
      name  = "serviceAccount.name"
      value = "aws-load-balancer-controller"
    },
    {
      name  = "serviceAccount.annotations.eks\\.amazonaws\\.com/role-arn"
      value = module.alb_controller_irsa.iam_role_arn
    },
  ]

  depends_on = [module.alb_controller_irsa]
}

resource "helm_release" "external_secrets" {
  name             = "external-secrets"
  repository       = "https://charts.external-secrets.io"
  chart            = "external-secrets"
  version          = var.external_secrets_chart_version
  namespace        = "external-secrets"
  create_namespace = true
  atomic           = true

  set = [
    {
      name  = "serviceAccount.annotations.eks\\.amazonaws\\.com/role-arn"
      value = module.external_secrets_irsa.iam_role_arn
    },
  ]

  # alb_controller installs a MutatingWebhookConfiguration that intercepts every
  # v1.Service create. ESO creates Services during install; if alb_controller
  # pods aren't Ready yet, those calls fail with "no endpoints available for
  # service aws-load-balancer-webhook-service". Serialise to avoid the race.
  depends_on = [
    module.external_secrets_irsa,
    helm_release.alb_controller,
  ]
}

resource "helm_release" "metrics_server" {
  name       = "metrics-server"
  repository = "https://kubernetes-sigs.github.io/metrics-server/"
  chart      = "metrics-server"
  version    = var.metrics_server_chart_version
  namespace  = "kube-system"
  atomic     = true

  # Same webhook race as external_secrets above — metrics-server also creates a
  # Service that the ALB controller webhook intercepts.
  depends_on = [helm_release.alb_controller]
}
