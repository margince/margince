# One alarm, for the one thing "light" actually needs watched: the single
# instance's own status checks. No CPU-credit-balance alarms the way the
# full stack's alarms.tf carries (rds.tf/elasticache.tf here have no
# Performance Insights/Enhanced Monitoring/slow-log to correlate against
# anyway) — this is the light/small-deployment option, and one signal that
# "the box is down" is the honest floor, not a smaller copy of the full
# stack's monitoring.
#
# Gated on var.enable_deep_monitoring, same reasoning as the full stack: no
# operator alert destination is picked on your own behalf. Subscribe with:
#   aws sns subscribe --topic-arn "$(terraform output -raw alerts_topic_arn)" \
#     --protocol email --notification-endpoint you@example.com

resource "aws_sns_topic" "alerts" {
  count = var.enable_deep_monitoring ? 1 : 0
  name  = "${var.name_prefix}-alerts"
  tags  = { Name = "${var.name_prefix}-alerts", Component = "observability" }
}

resource "aws_cloudwatch_metric_alarm" "instance_status_check_failed" {
  count               = var.enable_deep_monitoring ? 1 : 0
  alarm_name          = "${var.name_prefix}-ec2-status-check-failed"
  alarm_description   = "EC2 instance ${aws_instance.this.id} is failing its own status checks — the single point of failure this stack accepts, so this is the one signal that it's down."
  namespace           = "AWS/EC2"
  metric_name         = "StatusCheckFailed"
  dimensions          = { InstanceId = aws_instance.this.id }
  statistic           = "Maximum"
  period              = 60
  evaluation_periods  = 3
  threshold           = 0
  comparison_operator = "GreaterThanThreshold"
  treat_missing_data  = "breaching"

  alarm_actions = [aws_sns_topic.alerts[0].arn]
  ok_actions    = [aws_sns_topic.alerts[0].arn]

  tags = { Name = "${var.name_prefix}-ec2-status-check-failed", Component = "observability" }
}
