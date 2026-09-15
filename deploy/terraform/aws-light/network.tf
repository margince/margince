# "Light" cuts everything the full stack (deploy/terraform/aws/network.tf)
# spends on private-subnet compute: no NAT gateway (nothing here runs in a
# private subnet that needs internet egress — the one EC2 instance sits in
# the public subnet directly), no VPC Flow Logs, no VPC endpoints. Those are
# all real hardening the full stack correctly pays for; this stack's whole
# reason to exist is to not pay for them. RDS and ElastiCache still get
# private subnets — nothing on the public internet should ever reach either
# directly, EC2's own security group is the only path in.

data "aws_availability_zones" "available" {
  state = "available"
}

resource "aws_vpc" "this" {
  cidr_block           = var.vpc_cidr
  enable_dns_support   = true
  enable_dns_hostnames = true

  tags = { Name = "${var.name_prefix}-vpc" }
}

resource "aws_internet_gateway" "this" {
  vpc_id = aws_vpc.this.id
  tags   = { Name = "${var.name_prefix}-igw" }
}

# One public subnet — the EC2 instance's home, and the only place anything
# in this stack has a route to the internet gateway.
resource "aws_subnet" "public" {
  cidr_block              = cidrsubnet(var.vpc_cidr, 4, 0)
  vpc_id                  = aws_vpc.this.id
  availability_zone       = data.aws_availability_zones.available.names[0]
  map_public_ip_on_launch = true
  tags                    = { Name = "${var.name_prefix}-public" }
}

# RDS/ElastiCache subnet groups require two AZs even for a single-AZ
# instance/single-node cache — these carry no route to the IGW, so nothing
# in them is reachable from the internet regardless of any security group.
resource "aws_subnet" "private" {
  count             = var.az_count
  cidr_block        = cidrsubnet(var.vpc_cidr, 4, count.index + 1)
  vpc_id            = aws_vpc.this.id
  availability_zone = data.aws_availability_zones.available.names[count.index]
  tags              = { Name = "${var.name_prefix}-private-${count.index}" }
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
  subnet_id      = aws_subnet.public.id
  route_table_id = aws_route_table.public.id
}

# No route table association for the private subnets: the main route table
# AWS creates per-VPC (no explicit association) already covers them with a
# local-only route — no NAT, no IGW, no path out. That is deliberate, not an
# omission.

resource "aws_db_subnet_group" "this" {
  name       = "${var.name_prefix}-db"
  subnet_ids = aws_subnet.private[*].id
  tags       = { Name = "${var.name_prefix}-db", Component = "database" }
}

resource "aws_elasticache_subnet_group" "this" {
  name       = "${var.name_prefix}-redis"
  subnet_ids = aws_subnet.private[*].id
  tags       = { Name = "${var.name_prefix}-redis", Component = "cache" }
}

# ---- Security groups --------------------------------------------------------

# No SSH ingress on purpose: iam.tf attaches AmazonSSMManagedInstanceCore to
# the instance role, so operator shell access goes through SSM Session
# Manager (outbound-only, no listening port, every session logged) instead
# of an open port 22 anyone on the internet can attempt to reach.
resource "aws_security_group" "ec2" {
  name_prefix = "${var.name_prefix}-ec2-"
  description = "Single EC2 instance — HTTP(S) ingress from the internet to the nginx reverse-proxy container, egress to RDS/Redis/ECR/Secrets Manager/S3/the internet. No SSH ingress: shell access is via SSM Session Manager."
  vpc_id      = aws_vpc.this.id
  tags        = { Name = "${var.name_prefix}-ec2", Component = "network" }

  ingress {
    description = "HTTP from the internet — the ACME challenge + redirect-to-HTTPS when var.enable_tls is true (ec2.tf), or nginx's only listener when it's false."
    from_port   = 80
    to_port     = 80
    protocol    = "tcp"
    cidr_blocks = ["0.0.0.0/0"]
  }

  # Only opened when TLS is actually terminated here — var.enable_tls false
  # means nginx never binds :443 at all (nginx.conf.tftpl), so the port
  # would sit open with nothing listening behind it.
  dynamic "ingress" {
    for_each = var.enable_tls ? [1] : []
    content {
      description = "HTTPS from the internet, to nginx"
      from_port   = 443
      to_port     = 443
      protocol    = "tcp"
      cidr_blocks = ["0.0.0.0/0"]
    }
  }

  egress {
    description = "To RDS/ElastiCache in this VPC"
    from_port   = 0
    to_port     = 0
    protocol    = "-1"
    cidr_blocks = [var.vpc_cidr]
  }

  # Same egress shape as the full stack's ecs_tasks security group, and the
  # same reasoning: real feature surface (AI provider APIs, OAuth token
  # endpoints, outbound mail — docs/reference/configuration.md), plus what
  # the instance itself needs to reach ECR/Secrets Manager/S3 (no VPC
  # endpoints in this stack, see this file's own top comment — those calls
  # go out to the internet over these same HTTPS/443 rules).
  egress {
    description = "HTTPS to third-party APIs, ECR, Secrets Manager, S3"
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
  description = "RDS Postgres — ingress from the EC2 instance on 5432 only, no egress (RDS never originates outbound traffic)."
  vpc_id      = aws_vpc.this.id
  tags        = { Name = "${var.name_prefix}-db", Component = "database" }

  ingress {
    description     = "Postgres from the EC2 instance"
    from_port       = 5432
    to_port         = 5432
    protocol        = "tcp"
    security_groups = [aws_security_group.ec2.id]
  }

  lifecycle { create_before_destroy = true }
}

resource "aws_security_group" "redis" {
  name_prefix = "${var.name_prefix}-redis-"
  description = "ElastiCache — ingress from the EC2 instance on 6379 only, no egress (ElastiCache never originates outbound traffic)."
  vpc_id      = aws_vpc.this.id
  tags        = { Name = "${var.name_prefix}-redis", Component = "cache" }

  ingress {
    description     = "Redis from the EC2 instance"
    from_port       = 6379
    to_port         = 6379
    protocol        = "tcp"
    security_groups = [aws_security_group.ec2.id]
  }

  lifecycle { create_before_destroy = true }
}
