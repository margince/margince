# Path rules mirror docs/deployment.md's "Routing" table exactly: everything
# listed here goes to the api service; everything else, "/" included, is the
# listener's default action to the web (SPA) service.

# ---- Access log bucket -------------------------------------------------------
# Shared by the ALB's own access logs (this file) and the blobstore bucket's
# server access logs (s3.tf), under separate prefixes — both destinations
# have the identical constraint (same region, same account, SSE-S3 only, not
# this stack's own CMK: ALB access logging documents this explicitly,
# docs.aws.amazon.com/elasticloadbalancing/latest/application/enable-access-logging.html,
# "The only server-side encryption option that's supported is Amazon
# S3-managed keys (SSE-S3)"; S3 server access logging's own doc gives the
# identical requirement), so one bucket serves both rather than standing up
# a second one to hold nothing but a different prefix. This is why this
# bucket uses AES256 while every other bucket in this stack uses this
# stack's own CMK — not an inconsistency, a constraint of the two services
# being logged.
resource "aws_s3_bucket" "alb_logs" {
  bucket = "${var.name_prefix}-alb-logs"
}

resource "aws_s3_bucket_ownership_controls" "alb_logs" {
  bucket = aws_s3_bucket.alb_logs.id
  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

resource "aws_s3_bucket_public_access_block" "alb_logs" {
  bucket                  = aws_s3_bucket.alb_logs.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

resource "aws_s3_bucket_server_side_encryption_configuration" "alb_logs" {
  bucket = aws_s3_bucket.alb_logs.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

# Access logs are an audit trail, not application data — 90 days is enough to
# investigate an incident without keeping request logs forever. Same rule
# shape as s3.tf's noncurrent-version expiration, applied to current objects
# here since this bucket carries no versioning to begin with (nothing ever
# overwrites a delivered log file).
resource "aws_s3_bucket_lifecycle_configuration" "alb_logs" {
  bucket = aws_s3_bucket.alb_logs.id
  rule {
    id     = "expire-old-access-logs"
    status = "Enabled"
    filter {}
    expiration {
      days = 90
    }
  }
}

# Current, non-legacy bucket policy per the doc above: the log delivery
# service principal, not a region-specific numeric ELB account ID. The
# account-scoped resource path plus the aws:SourceArn condition are both the
# doc's own "security best practices" — the path alone stops a same-account
# bucket-name squatter, aws:SourceArn additionally stops any OTHER account's
# load balancer from writing here even if it somehow named this bucket.
data "aws_iam_policy_document" "alb_logs" {
  statement {
    sid     = "AWSLogDeliveryWrite"
    effect  = "Allow"
    actions = ["s3:PutObject"]
    principals {
      type        = "Service"
      identifiers = ["logdelivery.elasticloadbalancing.amazonaws.com"]
    }
    resources = ["${aws_s3_bucket.alb_logs.arn}/alb/AWSLogs/${data.aws_caller_identity.current.account_id}/*"]
    condition {
      test     = "ArnLike"
      variable = "aws:SourceArn"
      values   = ["arn:aws:elasticloadbalancing:${var.aws_region}:${data.aws_caller_identity.current.account_id}:loadbalancer/*"]
    }
  }

  # This bucket also carries s3.tf's blobstore access logs, under the "s3/"
  # prefix — same SSE-S3-only, same-region, same-account constraints as the
  # ALB logs above (docs.aws.amazon.com/AmazonS3/latest/userguide/enable-server-access-logging.html),
  # so one bucket serves both rather than standing up a second one to hold
  # nothing but a different prefix.
  statement {
    sid     = "S3ServerAccessLogsPolicy"
    effect  = "Allow"
    actions = ["s3:PutObject"]
    principals {
      type        = "Service"
      identifiers = ["logging.s3.amazonaws.com"]
    }
    resources = ["${aws_s3_bucket.alb_logs.arn}/s3/*"]
    condition {
      test     = "ArnLike"
      variable = "aws:SourceArn"
      values   = [aws_s3_bucket.blobstore.arn]
    }
    condition {
      test     = "StringEquals"
      variable = "aws:SourceAccount"
      values   = [data.aws_caller_identity.current.account_id]
    }
  }
}

resource "aws_s3_bucket_policy" "alb_logs" {
  bucket = aws_s3_bucket.alb_logs.id
  policy = data.aws_iam_policy_document.alb_logs.json
}

resource "aws_lb" "this" {
  name               = "${var.name_prefix}-alb"
  internal           = false
  load_balancer_type = "application"
  security_groups    = [aws_security_group.alb.id]
  subnets            = aws_subnet.public[*].id
  # A header with an invalid field name is otherwise forwarded to the target
  # as-is rather than dropped at the ALB — the parsing boundary this exists
  # to enforce (CWE-444, request smuggling via inconsistent interpretation).
  drop_invalid_header_fields = true

  # AWS's own default is false: a fat-fingered destroy or apply-time replace
  # would otherwise take down the one public entry point with no confirmation.
  enable_deletion_protection = true

  access_logs {
    bucket  = aws_s3_bucket.alb_logs.id
    prefix  = "alb"
    enabled = true
  }

  tags = { Name = "${var.name_prefix}-alb", Component = "edge" }

  # Every request this ALB ever serves is deniable by default until the
  # bucket policy above exists — ELB validates write access to the bucket at
  # creation/update time, not just at the first log flush.
  depends_on = [aws_s3_bucket_policy.alb_logs]
}

# HTTP, not HTTPS, from here to the ECS targets — deliberately, matching the
# product's own architecture: cmd/api serves plain HTTP and terminates TLS
# ahead of itself (docs/reference/configuration.md's --metrics-token row
# states this explicitly). TLS terminates at the ALB; the hop from here to
# the target stays inside this VPC's private subnets, never on the public
# internet. Re-encrypting it would ask the ECS targets to speak a protocol
# the api binary does not implement.
resource "aws_lb_target_group" "api" {
  name        = "${var.name_prefix}-api"
  port        = 8080
  protocol    = "HTTP"
  vpc_id      = aws_vpc.this.id
  target_type = "ip"

  health_check {
    path                = "/healthz"
    healthy_threshold   = 2
    unhealthy_threshold = 3
    interval            = 15
    timeout             = 5
    matcher             = "200"
  }

  tags = { Name = "${var.name_prefix}-api", Component = "edge" }
}

resource "aws_lb_target_group" "web" {
  name        = "${var.name_prefix}-web"
  port        = 8080
  protocol    = "HTTP"
  vpc_id      = aws_vpc.this.id
  target_type = "ip"

  health_check {
    path                = "/"
    healthy_threshold   = 2
    unhealthy_threshold = 3
    interval            = 15
    timeout             = 5
    matcher             = "200"
  }

  tags = { Name = "${var.name_prefix}-web", Component = "edge" }
}

resource "aws_lb_listener" "http_redirect" {
  load_balancer_arn = aws_lb.this.arn
  port              = 80
  protocol          = "HTTP"

  default_action {
    type = "redirect"
    redirect {
      port        = "443"
      protocol    = "HTTPS"
      status_code = "HTTP_301"
    }
  }
}

resource "aws_lb_listener" "https" {
  load_balancer_arn = aws_lb.this.arn
  port              = 443
  protocol          = "HTTPS"
  ssl_policy        = "ELBSecurityPolicy-TLS13-1-2-2021-06"
  certificate_arn   = var.acm_certificate_arn

  default_action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.web.arn
  }
}

resource "aws_lb_listener_rule" "api_v1_and_ops" {
  listener_arn = aws_lb_listener.https.arn
  priority     = 10

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.api.arn
  }

  condition {
    path_pattern {
      values = ["/v1*", "/healthz", "/readyz", "/metrics"]
    }
  }
}

resource "aws_lb_listener_rule" "api_webhooks" {
  listener_arn = aws_lb_listener.https.arn
  priority     = 20

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.api.arn
  }

  condition {
    path_pattern {
      values = ["/webhooks/gmail", "/webhooks/graph", "/webhooks/hubspot"]
    }
  }
}

resource "aws_lb_listener_rule" "api_mcp_oauth" {
  listener_arn = aws_lb_listener.https.arn
  priority     = 30

  action {
    type             = "forward"
    target_group_arn = aws_lb_target_group.api.arn
  }

  condition {
    path_pattern {
      values = [
        "/oauth/*",
        "/mcp*",
        "/.well-known/oauth-authorization-server",
        "/.well-known/oauth-protected-resource",
        "/.well-known/oauth-protected-resource/mcp",
      ]
    }
  }
}

# ---- WAF: the ALB is this stack's one public entry point --------------------
# AWS Managed Rules cover the exploit classes generic to any HTTP service
# (injection, known-bad payloads, known-malicious source IPs); the rate-based
# rule is this stack's own bound on request volume per client, independent of
# whatever limiting the api itself does at the application layer. Every rule
# runs in blocking mode (COUNT would log without protecting anything) — this
# is a stack that has no other layer in front of it to catch what these miss.
resource "aws_wafv2_web_acl" "alb" {
  name        = "${var.name_prefix}-alb"
  description = "Baseline managed-rule and rate-limit protection for ${var.name_prefix}'s public ALB"
  scope       = "REGIONAL"

  default_action {
    allow {}
  }

  rule {
    name     = "AWS-AWSManagedRulesCommonRuleSet"
    priority = 1
    override_action {
      none {}
    }
    statement {
      managed_rule_group_statement {
        name        = "AWSManagedRulesCommonRuleSet"
        vendor_name = "AWS"
      }
    }
    visibility_config {
      cloudwatch_metrics_enabled = true
      sampled_requests_enabled   = true
      metric_name                = "${var.name_prefix}-common-rule-set"
    }
  }

  rule {
    name     = "AWS-AWSManagedRulesKnownBadInputsRuleSet"
    priority = 2
    override_action {
      none {}
    }
    statement {
      managed_rule_group_statement {
        name        = "AWSManagedRulesKnownBadInputsRuleSet"
        vendor_name = "AWS"
      }
    }
    visibility_config {
      cloudwatch_metrics_enabled = true
      sampled_requests_enabled   = true
      metric_name                = "${var.name_prefix}-known-bad-inputs"
    }
  }

  rule {
    name     = "AWS-AWSManagedRulesAmazonIpReputationList"
    priority = 3
    override_action {
      none {}
    }
    statement {
      managed_rule_group_statement {
        name        = "AWSManagedRulesAmazonIpReputationList"
        vendor_name = "AWS"
      }
    }
    visibility_config {
      cloudwatch_metrics_enabled = true
      sampled_requests_enabled   = true
      metric_name                = "${var.name_prefix}-ip-reputation"
    }
  }

  # The api's own store is Postgres, reached through storekit's placeholder
  # derivation (AGENTS.md's "never hand-type a SQL placeholder") — this rule
  # group is the network-edge layer for the same class of attack that
  # invariant defends in the code, not a substitute for it.
  rule {
    name     = "AWS-AWSManagedRulesSQLiRuleSet"
    priority = 4
    override_action {
      none {}
    }
    statement {
      managed_rule_group_statement {
        name        = "AWSManagedRulesSQLiRuleSet"
        vendor_name = "AWS"
      }
    }
    visibility_config {
      cloudwatch_metrics_enabled = true
      sampled_requests_enabled   = true
      metric_name                = "${var.name_prefix}-sqli"
    }
  }

  # 2000 requests / 5-minute rolling window per client IP — generous enough
  # for a real user driving the SPA, tight enough to bound a single client
  # hammering the api. Evaluated at the ALB, ahead of any per-endpoint
  # rate limiting the api itself may apply.
  rule {
    name     = "RateLimitPerIP"
    priority = 5
    action {
      block {}
    }
    statement {
      rate_based_statement {
        limit              = 2000
        aggregate_key_type = "IP"
      }
    }
    visibility_config {
      cloudwatch_metrics_enabled = true
      sampled_requests_enabled   = true
      metric_name                = "${var.name_prefix}-rate-limit"
    }
  }

  # backend/internal/modules/identity/handlers.go's own limiters
  # (loginPerIP: 30/min, loginFailures: 10/min per email+IP) are, by their own
  # comment, "single-binary scope" — in-memory per api TASK, not shared
  # across the fleet. With api_desired_count/api_autoscaling_max_count
  # (variables.tf) putting 2-4 api tasks behind this ALB, an attacker's
  # requests spread across tasks by the ALB see up to N independent budgets,
  # not one shared one — the effective fleet-wide ceiling scales UP with
  # every task ECS adds, which is exactly backwards for a login endpoint.
  # This rule closes that gap the only place that sees traffic before it
  # fans out to any task at all: 100/5min (WAF's floor — rate_based_statement
  # can't go lower) is tighter per-IP than any single task's own 30/min, and
  # unlike the app's limiter, it holds regardless of fleet size.
  rule {
    name     = "RateLimitAuthPaths"
    priority = 6
    action {
      block {}
    }
    statement {
      rate_based_statement {
        limit              = 100
        aggregate_key_type = "IP"
        scope_down_statement {
          byte_match_statement {
            search_string         = "/v1/auth/"
            positional_constraint = "STARTS_WITH"
            field_to_match {
              uri_path {}
            }
            text_transformation {
              priority = 0
              type     = "NONE"
            }
          }
        }
      }
    }
    visibility_config {
      cloudwatch_metrics_enabled = true
      sampled_requests_enabled   = true
      metric_name                = "${var.name_prefix}-rate-limit-auth"
    }
  }

  visibility_config {
    cloudwatch_metrics_enabled = true
    sampled_requests_enabled   = true
    metric_name                = "${var.name_prefix}-alb-waf"
  }

  tags = { Name = "${var.name_prefix}-alb", Component = "edge" }
}

resource "aws_wafv2_web_acl_association" "alb" {
  resource_arn = aws_lb.this.arn
  web_acl_arn  = aws_wafv2_web_acl.alb.arn
}

# The "aws-waf-logs-" prefix is not stylistic — it is what lets WAF deliver to
# this log group at all (WAF only accepts a CloudWatch Logs destination whose
# name carries this exact prefix, no separate resource policy to maintain in
# step). Left on the account's default CloudWatch Logs encryption rather than
# this stack's own CMK — extending the CMK's key policy to the logs.amazonaws.com
# service principal is a separate, larger change than "log what WAF blocks",
# and every other CloudWatch Logs group in this stack (iam.tf) is unencrypted
# by the same default already.
resource "aws_cloudwatch_log_group" "waf" {
  name              = "aws-waf-logs-${var.name_prefix}"
  retention_in_days = var.log_retention_days
  tags              = { Name = "${var.name_prefix}-waf-logs", Component = "observability" }
}

resource "aws_wafv2_web_acl_logging_configuration" "alb" {
  resource_arn            = aws_wafv2_web_acl.alb.arn
  log_destination_configs = [aws_cloudwatch_log_group.waf.arn]

  # WAF logs the full request by default — headers included. Without this,
  # every session cookie and bearer token that ever crossed the ALB sits in
  # plaintext in a CloudWatch Logs group retained for
  # var.log_retention_days, readable by anyone with logs:GetLogEvents on it.
  # Redacting here doesn't stop the request from being evaluated (WAF still
  # sees the real header when deciding allow/block); it only replaces the
  # value with REDACTED in what gets written to aws_cloudwatch_log_group.waf.
  redacted_fields {
    single_header {
      name = "authorization"
    }
  }
  redacted_fields {
    single_header {
      name = "cookie"
    }
  }
}
