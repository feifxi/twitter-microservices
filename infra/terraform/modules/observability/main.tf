# amazon-cloudwatch-observability EKS managed addon installs the CloudWatch
# agent (Container Insights metrics) + Fluent Bit (container logs to CW Logs)
# in one bundle. Replaces the older standalone helm dance. Agent config below
# also enables Prometheus scraping of /metrics on every pod that opts in via
# pod annotation `scrape: "true"` — feeds app-level metrics into CW under the
# ContainerInsights/Prometheus namespace.

module "cloudwatch_agent_irsa" {
  source  = "terraform-aws-modules/iam/aws//modules/iam-role-for-service-accounts-eks"
  version = "~> 5.50"

  role_name = "${var.name}-cloudwatch-agent"
  role_policy_arns = {
    cw_agent   = "arn:aws:iam::aws:policy/CloudWatchAgentServerPolicy"
    xray_write = "arn:aws:iam::aws:policy/AWSXRayDaemonWriteAccess"
  }

  oidc_providers = {
    main = {
      provider_arn               = var.oidc_provider_arn
      namespace_service_accounts = ["amazon-cloudwatch:cloudwatch-agent"]
    }
  }

  tags = var.tags
}

resource "aws_eks_addon" "cloudwatch_observability" {
  cluster_name                = var.cluster_name
  addon_name                  = "amazon-cloudwatch-observability"
  service_account_role_arn    = module.cloudwatch_agent_irsa.iam_role_arn
  resolve_conflicts_on_create = "OVERWRITE"
  resolve_conflicts_on_update = "OVERWRITE"

  # Prometheus scrape pointed at the apps namespace. Picks up any pod that
  # exposes a port named "metrics" and lands the samples in CloudWatch as EMF
  # under the ContainerInsights/Prometheus metric namespace.
  configuration_values = jsonencode({
    agent = {
      config = {
        logs = {
          metrics_collected = {
            prometheus = {
              prometheus_config_path = "/etc/prometheusconfig/prometheus.yaml"
              emf_processor = {
                metric_declaration = [{
                  source_labels = ["job"]
                  label_matcher = "twitter-apps"
                  dimensions    = [["ClusterName", "Namespace", "service"]]
                  metric_selectors = [
                    "^outbox_pending_count$",
                    "^http_requests_total$",
                    "^http_request_duration_seconds.*",
                  ]
                }]
              }
            }
          }
        }
      }
    }
    containerLogs = {
      enabled = true
    }
  })

  tags = var.tags
}

# Single SNS topic for every alarm. Subscriber confirms via email link AWS
# sends after first apply; until confirmed, no notifications fire.
resource "aws_sns_topic" "alarms" {
  name = "${var.name}-alarms"
  tags = var.tags
}

resource "aws_sns_topic_subscription" "email" {
  topic_arn = aws_sns_topic.alarms.arn
  protocol  = "email"
  endpoint  = var.alarm_email
}

# Alarm 1: ALB target 5xx > 10 / minute
# Native ALB CloudWatch metric, no app instrumentation needed.
resource "aws_cloudwatch_metric_alarm" "alb_5xx" {
  alarm_name          = "${var.name}-alb-5xx"
  alarm_description   = "ALB target group returned 5xx more than 10 times in the last minute."
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 1
  metric_name         = "HTTPCode_Target_5XX_Count"
  namespace           = "AWS/ApplicationELB"
  period              = 60
  statistic           = "Sum"
  threshold           = 10
  treat_missing_data  = "notBreaching"
  alarm_actions       = [aws_sns_topic.alarms.arn]
  ok_actions          = [aws_sns_topic.alarms.arn]

  dimensions = {
    LoadBalancer = var.alb_arn_suffix
  }

  tags = var.tags
}

# Alarm 2: Kafka consumer lag > 30s (proxy: max offset lag > 1000 messages,
# tunable). MaxOffsetLag is the per-consumer-group rollup MSK emits without
# enabling open monitoring (which would cost extra).
resource "aws_cloudwatch_metric_alarm" "kafka_lag" {
  alarm_name          = "${var.name}-kafka-consumer-lag"
  alarm_description   = "MSK consumer-group offset lag breached 1000 messages, sustained for 3 minutes."
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 3
  metric_name         = "MaxOffsetLag"
  namespace           = "AWS/Kafka"
  period              = 60
  statistic           = "Maximum"
  threshold           = 1000
  treat_missing_data  = "notBreaching"
  alarm_actions       = [aws_sns_topic.alarms.arn]
  ok_actions          = [aws_sns_topic.alarms.arn]

  dimensions = {
    "Cluster Name" = var.msk_cluster_name
  }

  tags = var.tags
}

# Alarm 3: outbox depth growing. Requires services to expose
# `outbox_pending_count` on their /metrics endpoint AND the prometheus scrape
# above to pick it up. Fires when any service's outbox has > 100 unsent rows
# for 5 minutes — symptom of the outbox publisher being stuck.
resource "aws_cloudwatch_metric_alarm" "outbox_depth" {
  alarm_name          = "${var.name}-outbox-depth"
  alarm_description   = "Outbox has > 100 unsent rows for 5 minutes. Publisher goroutine likely stuck or Kafka producer failing."
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 5
  metric_name         = "outbox_pending_count"
  namespace           = "ContainerInsights/Prometheus"
  period              = 60
  statistic           = "Maximum"
  threshold           = 100
  treat_missing_data  = "notBreaching"
  alarm_actions       = [aws_sns_topic.alarms.arn]
  ok_actions          = [aws_sns_topic.alarms.arn]

  dimensions = {
    ClusterName = var.cluster_name
    Namespace   = var.apps_namespace
  }

  tags = var.tags
}
