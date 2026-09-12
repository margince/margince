data "aws_availability_zones" "available" {
  state = "available"
}

resource "aws_vpc" "this" {
  cidr_block           = var.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = { Name = "${var.name_prefix}-vpc" }
}

# ---- VPC Flow Logs -----------------------------------------------------------
# Every security group in this file (alb/ecs_tasks/db/redis/efs/vpc_endpoints)
# is a set of claims about what traffic is allowed — nothing in this stack
# records what traffic actually FLOWED, accepted or rejected, until this.
# Without it, "was this SG rule ever hit" or "what tried to reach the db SG
# and got refused" during an incident has no answer at all.
resource "aws_cloudwatch_log_group" "vpc_flow_logs" {
  name              = "/vpc-flow-logs/${var.name_prefix}"
  retention_in_days = var.log_retention_days
  tags              = { Name = "${var.name_prefix}-vpc-flow-logs", Component = "observability" }
}

data "aws_iam_policy_document" "vpc_flow_logs_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["vpc-flow-logs.amazonaws.com"]
    }
    # Same confused-deputy reasoning as iam.tf's ecs_assume: a bare service
    # principal trusts vpc-flow-logs.amazonaws.com everywhere, not just this
    # account's own flow logs delivering to this role.
    condition {
      test     = "StringEquals"
      variable = "aws:SourceAccount"
      values   = [data.aws_caller_identity.current.account_id]
    }
    condition {
      test     = "ArnLike"
      variable = "aws:SourceArn"
      values   = ["arn:aws:ec2:${var.aws_region}:${data.aws_caller_identity.current.account_id}:vpc-flow-log/*"]
    }
  }
}

resource "aws_iam_role" "vpc_flow_logs" {
  name               = "${var.name_prefix}-vpc-flow-logs"
  assume_role_policy = data.aws_iam_policy_document.vpc_flow_logs_assume.json
  tags               = { Name = "${var.name_prefix}-vpc-flow-logs", Component = "security" }
}

data "aws_iam_policy_document" "vpc_flow_logs_delivery" {
  statement {
    sid     = "WriteFlowLogs"
    actions = ["logs:CreateLogStream", "logs:PutLogEvents", "logs:DescribeLogGroups", "logs:DescribeLogStreams"]
    resources = [
      aws_cloudwatch_log_group.vpc_flow_logs.arn,
      "${aws_cloudwatch_log_group.vpc_flow_logs.arn}:*",
    ]
  }
}

resource "aws_iam_role_policy" "vpc_flow_logs_delivery" {
  name   = "${var.name_prefix}-vpc-flow-logs-delivery"
  role   = aws_iam_role.vpc_flow_logs.id
  policy = data.aws_iam_policy_document.vpc_flow_logs_delivery.json
}

resource "aws_flow_log" "this" {
  vpc_id               = aws_vpc.this.id
  traffic_type         = "ALL"
  log_destination_type = "cloud-watch-logs"
  log_destination      = aws_cloudwatch_log_group.vpc_flow_logs.arn
  iam_role_arn         = aws_iam_role.vpc_flow_logs.arn
  tags                 = { Name = "${var.name_prefix}-vpc-flow-log", Component = "observability" }
}

resource "aws_internet_gateway" "this" {
  vpc_id = aws_vpc.this.id
  tags   = { Name = "${var.name_prefix}-igw" }
}

resource "aws_subnet" "public" {
  count                   = var.az_count
  vpc_id                  = aws_vpc.this.id
  cidr_block              = cidrsubnet(var.vpc_cidr, 4, count.index)
  availability_zone       = data.aws_availability_zones.available.names[count.index]
  map_public_ip_on_launch = true
  tags                    = { Name = "${var.name_prefix}-public-${count.index}" }
}

resource "aws_subnet" "private" {
  count             = var.az_count
  vpc_id            = aws_vpc.this.id
  cidr_block        = cidrsubnet(var.vpc_cidr, 4, var.az_count + count.index)
  availability_zone = data.aws_availability_zones.available.names[count.index]
  tags              = { Name = "${var.name_prefix}-private-${count.index}" }
}

resource "aws_eip" "nat" {
  count  = var.az_count
  domain = "vpc"
  tags   = { Name = "${var.name_prefix}-nat-${count.index}" }
}

# One NAT gateway per AZ: private-subnet tasks (ECS pulling images, RDS/Redis
# reached only from here) need egress for the ECR/Secrets Manager API calls
# ECS makes on their behalf, and a single shared NAT would make every AZ
# depend on one that isn't its own.
resource "aws_nat_gateway" "this" {
  count         = var.az_count
  allocation_id = aws_eip.nat[count.index].id
  subnet_id     = aws_subnet.public[count.index].id
  tags          = { Name = "${var.name_prefix}-nat-${count.index}" }

  depends_on = [aws_internet_gateway.this]
}

resource "aws_route_table" "public" {
  vpc_id = aws_vpc.this.id
  route {
    cidr_block = "0.0.0.0/0"
    gateway_id = aws_internet_gateway.this.id
  }
  tags = { Name = "${var.name_prefix}-public" }
}

resource "aws_route_table_association" "public" {
  count          = var.az_count
  subnet_id      = aws_subnet.public[count.index].id
  route_table_id = aws_route_table.public.id
}

resource "aws_route_table" "private" {
  count  = var.az_count
  vpc_id = aws_vpc.this.id
  route {
    cidr_block     = "0.0.0.0/0"
    nat_gateway_id = aws_nat_gateway.this[count.index].id
  }
  tags = { Name = "${var.name_prefix}-private-${count.index}" }
}

resource "aws_route_table_association" "private" {
  count          = var.az_count
  subnet_id      = aws_subnet.private[count.index].id
  route_table_id = aws_route_table.private[count.index].id
}

# ---- Security groups --------------------------------------------------------

resource "aws_security_group" "alb" {
  name_prefix = "${var.name_prefix}-alb-"
  vpc_id      = aws_vpc.this.id
  tags        = { Name = "${var.name_prefix}-alb", Component = "network" }

  ingress {
    description = "HTTPS from the internet"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # aws_lb_listener.http_redirect (alb.tf) answers on :80 with nothing but a
  # 301 to :443 — never forwards to a target — but it still needs an inbound
  # rule of its own, or the redirect itself is unreachable and every plain
  # http:// request just times out instead of being sent to https://.
  ingress {
    description = "HTTP from the internet, for the redirect to HTTPS only"
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # The ALB only ever originates traffic to the api/web target groups inside
  # this VPC — 0.0.0.0/0 egress bought it nothing but a wider blast radius.
  egress {
    description = "To ECS targets in this VPC"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = [var.vpc_cidr]
  }

  lifecycle { create_before_destroy = true }
}

resource "aws_security_group" "ecs_tasks" {
  name_prefix = "${var.name_prefix}-ecs-"
  vpc_id      = aws_vpc.this.id
  tags        = { Name = "${var.name_prefix}-ecs-tasks", Component = "network" }

  ingress {
    description     = "ALB to api/web containers"
    from_port       = 8080
    to_port         = 8080
    protocol        = "tcp"
    security_groups = [aws_security_group.alb.id]
  }

  # Was 0.0.0.0/0 on all ports/protocols — a compromised task could open a
  # connection to anywhere, on any port, which is a bigger blast radius than
  # anything this app actually needs. Two rules instead:
  #
  # 1. Everything IN this VPC, every port: RDS (5432), ElastiCache (6379),
  #    EFS (2049), the VPC endpoints below (443), and the VPC's own DNS
  #    resolver — all genuinely used, none of them worth naming one port
  #    at a time when they already share one trust boundary.
  egress {
    description = "To everything in this VPC"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = [var.vpc_cidr]
  }
  # 2. Specific ports the app genuinely calls out to the internet for, and
  #    nothing else: HTTPS (AI provider APIs, Nominatim, VIES, crt.sh, OAuth
  #    token endpoints, license validation — see docs/reference/configuration.md)
  #    and outbound mail (SMTP submission/implicit-TLS/plain, since an
  #    operator's relay's port depends on what they configured under
  #    email.smtp). This is real feature surface, not a gap left open by
  #    oversight — narrowing it further would break documented capabilities
  #    this stack does not get to disable on an operator's behalf.
  egress {
    description = "HTTPS to third-party APIs this app calls (see comment)"
    from_port   = 443
    to_port     = 443
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
  egress {
    description = "Outbound mail relay (SMTP submission/implicit-TLS/plain)"
    from_port   = 25
    to_port     = 25
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
  egress {
    from_port   = 465
    to_port     = 465
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }
  egress {
    from_port   = 587
    to_port     = 587
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  lifecycle { create_before_destroy = true }
}

resource "aws_security_group" "db" {
  name_prefix = "${var.name_prefix}-db-"
  vpc_id      = aws_vpc.this.id
  tags        = { Name = "${var.name_prefix}-db", Component = "database" }

  ingress {
    description     = "Postgres from ECS tasks"
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.ecs_tasks.id]
  }

  # No egress block, deliberately: RDS never originates an outbound
  # connection, so "allow all outbound" bought this SG nothing but blast
  # radius if the instance were ever compromised. Terraform manages the full
  # rule set for aws_security_group — omitting egress here revokes the
  # allow-all rule AWS creates by default at CreateSecurityGroup, leaving
  # zero egress permitted rather than a rule nobody meant to grant.

  lifecycle { create_before_destroy = true }
}

resource "aws_security_group" "redis" {
  name_prefix = "${var.name_prefix}-redis-"
  vpc_id      = aws_vpc.this.id
  tags        = { Name = "${var.name_prefix}-redis", Component = "cache" }

  ingress {
    description     = "Redis from ECS tasks"
    from_port       = 6379
    to_port         = 6379
    protocol        = "tcp"
    security_groups = [aws_security_group.ecs_tasks.id]
  }

  # No egress block — see aws_security_group.db's comment; ElastiCache never
  # originates outbound traffic either.

  lifecycle { create_before_destroy = true }
}

resource "aws_security_group" "efs" {
  name_prefix = "${var.name_prefix}-efs-"
  vpc_id      = aws_vpc.this.id
  tags        = { Name = "${var.name_prefix}-efs", Component = "storage" }

  ingress {
    description     = "NFS from ECS tasks"
    from_port       = 2049
    to_port         = 2049
    protocol        = "tcp"
    security_groups = [aws_security_group.ecs_tasks.id]
  }

  # No egress block — see aws_security_group.db's comment; EFS mount targets
  # never originate outbound traffic either.

  lifecycle { create_before_destroy = true }
}

resource "aws_db_subnet_group" "this" {
  name       = "${var.name_prefix}-db"
  subnet_ids = aws_subnet.private[*].id
  tags       = { Name = "${var.name_prefix}-db", Component = "database" }
}

resource "aws_elasticache_subnet_group" "this" {
  name       = "${var.name_prefix}-redis"
  subnet_ids = aws_subnet.private[*].id
}
