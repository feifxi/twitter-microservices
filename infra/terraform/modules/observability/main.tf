# amazon-cloudwatch-observability EKS managed addon installs the CloudWatch
# agent (Container Insights metrics) + Fluent Bit (container logs to CW Logs)
# in one bundle. Replaces the older standalone helm dance.
#
# Container Insights gives pod/node CPU + memory + network out of the box.
# App-level metrics (Prometheus /metrics) are not scraped by this addon — the
# CW Agent Operator does that, deferred to the scale-out path.

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
  tags                        = var.tags
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

# Alarm 1: ALB target 5xx > 10 / minute.
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

# Alarm 2: MSK consumer lag > 1000 messages sustained 3 min.
# MaxOffsetLag is the per-broker rollup MSK emits without enabling open
# monitoring (Prometheus exporter would cost extra).
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

# Alarm 3: Aurora CPU > 80% sustained 5 min.
# Native RDS metric. Catches runaway queries / outbox publisher hot-looping /
# unindexed scans before they cascade into latency on dependent services.
resource "aws_cloudwatch_metric_alarm" "aurora_cpu" {
  alarm_name          = "${var.name}-aurora-cpu"
  alarm_description   = "Aurora cluster CPU > 80% for 5 minutes. Check slow query log."
  comparison_operator = "GreaterThanThreshold"
  evaluation_periods  = 5
  metric_name         = "CPUUtilization"
  namespace           = "AWS/RDS"
  period              = 60
  statistic           = "Average"
  threshold           = 80
  treat_missing_data  = "notBreaching"
  alarm_actions       = [aws_sns_topic.alarms.arn]
  ok_actions          = [aws_sns_topic.alarms.arn]

  dimensions = {
    DBClusterIdentifier = var.aurora_cluster_identifier
  }

  tags = var.tags
}
