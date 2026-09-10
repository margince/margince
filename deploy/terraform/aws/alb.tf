# Path rules mirror docs/deployment.md's "Routing" table exactly: everything
# listed here goes to the api service; everything else, "/" included, is the
# listener's default action to the web (SPA) service.

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
