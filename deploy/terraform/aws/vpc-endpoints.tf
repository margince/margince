# Keeps ECS→AWS-API traffic (ECR pulls, Secrets Manager reads, KMS decrypts,
# CloudWatch Logs writes) inside the VPC instead of round-tripping through the
# NAT gateway and the public internet path — the same traffic the ecs_tasks
# security group's 0.0.0.0/0:443 egress rule (network.tf) would otherwise
# carry there. Narrowing that egress rule bought nothing on its own if every
# AWS SDK call still had to leave the VPC to reach it; these endpoints are
# what makes the narrowing actually change the traffic's path, not just its
# security-group accounting.
#
# Every endpoint below gets the SAME restriction: only THIS account's own IAM
# principals may use it, full stop — actions and resources are left to IAM
# (iam.tf already scopes those tightly, per role; duplicating that scoping
# here would be a second copy of the same invariant, drifting the moment one
# side changes without the other). What an endpoint policy adds that IAM
# alone can't is a floor under a credentials-compromise scenario an IAM
# policy never reaches: if a task somehow ended up holding another account's
# credentials (a copy-pasted key, a supply-chain compromise), those
# credentials could still authenticate to AWS — but this condition refuses
# them at the endpoint before the call ever reaches the service, because
# they're not THIS account's principal. This is deliberately NOT
# s3:ResourceAccount-style resource restriction on the S3 endpoint — this
# same Gateway endpoint carries ECR's own image-layer blob storage (comment
# below), which lives in AWS-owned buckets outside this account entirely;
# restricting by resource account would break every image pull.
data "aws_iam_policy_document" "vpc_endpoint_same_account_only" {
  statement {
    effect  = "Allow"
    actions = ["*"]
    principals {
      type        = "AWS"
      identifiers = ["*"]
    }
    resources = ["*"]
    condition {
      test     = "StringEquals"
      variable = "aws:PrincipalAccount"
      values   = [data.aws_caller_identity.current.account_id]
    }
  }
}

# S3 is a Gateway endpoint (route-table based, no hourly cost, no ENI) — used
# by the ECR interface endpoints below for the actual image-layer blob
# storage backing ECR, which the interface endpoint alone does not cover.
resource "aws_vpc_endpoint" "s3" {
  vpc_id            = aws_vpc.this.id
  service_name      = "com.amazonaws.${var.aws_region}.s3"
  vpc_endpoint_type = "Gateway"
  route_table_ids   = aws_route_table.private[*].id
  policy            = data.aws_iam_policy_document.vpc_endpoint_same_account_only.json
  tags              = { Name = "${var.name_prefix}-s3", Component = "network" }
}

resource "aws_security_group" "vpc_endpoints" {
  name_prefix = "${var.name_prefix}-vpce-"
  vpc_id      = aws_vpc.this.id
  tags        = { Name = "${var.name_prefix}-vpce", Component = "network" }

  ingress {
    description     = "HTTPS from ECS tasks"
    from_port       = 443
    to_port         = 443
    protocol        = "tcp"
    security_groups = [aws_security_group.ecs_tasks.id]
  }

  # No egress block — an interface endpoint's ENI answers requests, it never
  # originates one (same reasoning as aws_security_group.db/.redis/.efs).

  lifecycle { create_before_destroy = true }
}

locals {
  # One list, one resource block (for_each below) rather than five nearly
  # identical resources — the five services need nothing different from each
  # other: same subnets, same security group, same private-DNS setting.
  interface_endpoint_services = toset([
    "ecr.api",
    "ecr.dkr",
    "secretsmanager",
    "kms",
    "logs",
  ])
}

resource "aws_vpc_endpoint" "interface" {
  for_each = local.interface_endpoint_services

  vpc_id              = aws_vpc.this.id
  service_name        = "com.amazonaws.${var.aws_region}.${each.value}"
  vpc_endpoint_type   = "Interface"
  subnet_ids          = aws_subnet.private[*].id
  security_group_ids  = [aws_security_group.vpc_endpoints.id]
  private_dns_enabled = true
  policy              = data.aws_iam_policy_document.vpc_endpoint_same_account_only.json
  tags                = { Name = "${var.name_prefix}-${each.value}", Component = "network" }
}
