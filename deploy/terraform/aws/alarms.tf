# CPUCreditBalance alarms for the two burstable (T-family) instance types this
# stack defaults to — db.t4g.medium (rds.tf) and cache.t4g.small
# (elasticache.tf). Both accumulate credits at idle and can throttle under
# SUSTAINED load once the balance is exhausted; that throttling shows up as
# "the app got slow" with nothing else obviously wrong, unless something is
# watching the balance itself. Performance Insights/Enhanced Monitoring
# (rds.tf) and the slow-log export (elasticache.tf) tell you AFTER the fact
# that something was slow — these alarms are the one signal that fires BEFORE
# a burstable instance actually throttles.
#
# No subscription is created here — an operator's alert destination (email,
# Slack, PagerDuty) is theirs to own, the same reasoning this stack gives for
# every other operator-specific value it declines to pick on your behalf.
# Subscribe with:
#   aws sns subscribe --topic-arn "$(terraform output -raw alerts_topic_arn)" \
#     --protocol email --notification-endpoint you@example.com

resource "aws_sns_topic" "alerts" {
  name              = "${var.name_prefix}-alerts"
  kms_master_key_id = aws_kms_key.data.arn
  tags              = { Name = "${var.name_prefix}-alerts", Component = "observability" }
}

resource "aws_cloudwatch_metric_alarm" "rds_cpu_credit_balance" {
  alarm_name         = "${var.name_prefix}-rds-cpu-credit-balance-low"
  alarm_description  = "RDS ${aws_db_instance.this.identifier} is burning through its CPU credit balance — sustained load is about to throttle it, not a transient spike."
  namespace          = "AWS/RDS"
  metric_name        = "CPUCreditBalance"
  dimensions         = { DBInstanceIdentifier = aws_db_instance.this.id }
  statistic          = "Average"
  period             = 300
  evaluation_periods = 3
  # A starting point, not a tuned value — the honest floor before any real
  # load data exists (this stack's own README says the same about instance
  # sizing). Revisit once Performance Insights shows what this instance's
  # actual credit consumption looks like under real traffic.
  threshold           = 20
  comparison_operator = "LessThanThreshold"
  treat_missing_data  = "breaching"

  alarm_actions = [aws_sns_topic.alerts.arn]
  ok_actions    = [aws_sns_topic.alerts.arn]

  tags = { Name = "${var.name_prefix}-rds-cpu-credit-balance", Component = "observability" }
}

locals {
  # The one place this number is spelled — elasticache.tf's
  # num_cache_clusters reads it too, so the replication group's actual node
  # count and the per-node alarms below can never silently disagree.
  redis_node_count = 2
}

# One alarm per node: CPUCreditBalance is a per-node metric (CacheClusterId
# dimension), and ElastiCache assigns member node IDs
# "<replication_group_id>-001".."-00N" when num_cache_clusters is set without
# an explicit cluster_mode/preferred_availability_zones override — the
# convention aws_elasticache_replication_group.this already relies on
# implicitly, made explicit here because the alarm has to name the node.
resource "aws_cloudwatch_metric_alarm" "redis_cpu_credit_balance" {
  count               = local.redis_node_count
  alarm_name          = "${var.name_prefix}-redis-cpu-credit-balance-low-${count.index + 1}"
  alarm_description   = "ElastiCache node ${count.index + 1} of ${aws_elasticache_replication_group.this.replication_group_id} is burning through its CPU credit balance."
  namespace           = "AWS/ElastiCache"
  metric_name         = "CPUCreditBalance"
  dimensions          = { CacheClusterId = "${aws_elasticache_replication_group.this.replication_group_id}-${format("%03d", count.index + 1)}" }
  statistic           = "Average"
  period              = 300
  evaluation_periods  = 3
  threshold           = 20
  comparison_operator = "LessThanThreshold"
  treat_missing_data  = "breaching"

  alarm_actions = [aws_sns_topic.alerts.arn]
  ok_actions    = [aws_sns_topic.alerts.arn]

  tags = { Name = "${var.name_prefix}-redis-cpu-credit-balance-${count.index + 1}", Component = "observability" }
}
