output "sns_topic_arn" {
  description = "SNS topic that fans out every alarm. Subscribe additional protocols (Slack via Lambda, PagerDuty, etc.) to this ARN."
  value       = aws_sns_topic.alarms.arn
}

output "alarm_arns" {
  description = "Map of alarm short-name -> ARN. Handy for cross-referencing in dashboards."
  value = {
    alb_5xx      = aws_cloudwatch_metric_alarm.alb_5xx.arn
    kafka_lag    = aws_cloudwatch_metric_alarm.kafka_lag.arn
    outbox_depth = aws_cloudwatch_metric_alarm.outbox_depth.arn
  }
}
