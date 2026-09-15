# Same third-credential reasoning as the full stack (deploy/terraform/aws/rds.tf):
# "dbadmin" is RDS's own master user, distinct from the two DB roles
# scripts/deploy/db-bootstrap.sql creates (margince_owner, margince_app).

resource "random_password" "rds_master" {
  length  = 32
  special = false
}

resource "random_password" "margince_owner" {
  length  = 32
  special = false
}

resource "random_password" "margince_app" {
  length  = 32
  special = false
}

resource "random_id" "final_snapshot" {
  byte_length = 4
  keepers = {
    generation = var.db_final_snapshot_generation
  }
}

# rds.force_ssl is the security floor this stack keeps even though it drops
# the full stack's customer-managed KMS key — encryption-at-rest defaults to
# AWS-managed (storage_encrypted below, no kms_key_id), but encryption/
# authentication IN TRANSIT to the database is not a place "light" cuts
# corners. Same reasoning as the full stack: sslmode=prefer/require alone
# lets a client fall back to (require) or fail to verify (prefer) plaintext
# or an unauthenticated server; force_ssl makes the SERVER refuse a non-TLS
# connection outright.
resource "aws_db_parameter_group" "this" {
  name_prefix = "${var.name_prefix}-pg16-"
  family      = "postgres16"

  parameter {
    name         = "rds.force_ssl"
    value        = "1"
    apply_method = "pending-reboot"
  }

  parameter {
    name         = "log_min_duration_statement"
    value        = "1000"
    apply_method = "immediate"
  }
  parameter {
    name         = "log_connections"
    value        = "1"
    apply_method = "immediate"
  }
  parameter {
    name         = "log_disconnections"
    value        = "1"
    apply_method = "immediate"
  }
  parameter {
    name         = "log_lock_waits"
    value        = "1"
    apply_method = "immediate"
  }

  tags = { Name = "${var.name_prefix}-pg16", Component = "database" }

  lifecycle { create_before_destroy = true }
}

resource "aws_cloudwatch_log_group" "rds_postgresql" {
  name              = "/aws/rds/instance/${var.name_prefix}-db/postgresql"
  retention_in_days = var.log_retention_days
  tags              = { Name = "${var.name_prefix}-db-postgresql-logs", Component = "observability" }
}

resource "aws_db_instance" "this" {
  identifier     = "${var.name_prefix}-db"
  engine         = "postgres"
  engine_version = var.db_engine_version
  instance_class = var.db_instance_class

  allocated_storage     = var.db_allocated_storage_gb
  max_allocated_storage = var.db_allocated_storage_gb * 4
  storage_type          = "gp3"
  # AWS-managed key (no kms_key_id), not a customer-managed CMK — "light"
  # means one fewer credential to grant and rotate; storage is still
  # encrypted, just under the default aws/rds key rather than a key this
  # stack owns and could independently revoke.
  storage_encrypted = true

  db_name  = "margince"
  username = "dbadmin"
  password = random_password.rds_master.result
  port     = 5432

  parameter_group_name   = aws_db_parameter_group.this.name
  db_subnet_group_name   = aws_db_subnet_group.this.name
  vpc_security_group_ids = [aws_security_group.db.id]
  publicly_accessible    = false

  ca_cert_identifier = "rds-ca-rsa2048-g1"

  # Single-AZ, on purpose: this is the whole point of "light" on the compute
  # side extended to the database too — one instance, no standby, cheaper,
  # and a failure means restoring from backup rather than an automatic
  # failover. If that recovery time isn't acceptable, use the full stack
  # (deploy/terraform/aws/) instead, not this one with multi_az bolted on.
  multi_az                = false
  backup_retention_period = var.db_backup_retention_days
  backup_window           = "03:00-04:00"
  maintenance_window      = "mon:04:30-mon:05:30"
  copy_tags_to_snapshot   = true

  # No Performance Insights, no Enhanced Monitoring: both are real
  # observability the full stack correctly pays for; db.t4g.micro's own
  # CloudWatch basic metrics (free, 5-minute granularity) are the floor this
  # stack ships with instead.
  enabled_cloudwatch_logs_exports = ["postgresql"]

  # deletion_protection false and skip_final_snapshot false: an operator
  # tearing down a light/dev-shaped stack should be able to `terraform
  # destroy` without a separate console step to disable protection first,
  # while still keeping one final snapshot as the last recovery point.
  deletion_protection       = false
  skip_final_snapshot       = false
  final_snapshot_identifier = "${var.name_prefix}-db-final-${random_id.final_snapshot.hex}"

  tags = { Name = "${var.name_prefix}-db", Component = "database" }

  # scripts/deploy/db-bootstrap.sql runs once, by hand, against this
  # instance as "dbadmin" — see this stack's README. Unlike the full stack,
  # there's no separate bastion step: the EC2 instance this stack creates
  # already sits in the same VPC and can reach the RDS private endpoint
  # directly.
}
