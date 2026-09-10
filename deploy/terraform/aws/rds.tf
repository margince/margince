# The RDS instance's own master user is a THIRD credential, distinct from the
# two DB roles scripts/deploy/db-bootstrap.sql creates (margince_owner,
# margince_app — see docs/deployment.md's "two-role database model"). It is
# named "dbadmin" rather than "margince_owner" specifically so it is never
# confused with the role the bootstrap script creates: this one is RDS's own
# master user (rds_superuser-equivalent, needed once to run db-bootstrap.sql,
# which installs pgvector — an untrusted extension only a superuser-equivalent
# role can install), the other is the non-superuser role the api/migrate
# connect as afterwards.
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

# storage_encrypted below protects the disk; it says nothing about the wire.
# Without this, a client can open a plaintext TCP connection to Postgres and
# RDS will serve it — pgx/libpq default to sslmode=prefer, which attempts TLS
# but silently falls back to plaintext rather than refusing, so the DSN alone
# cannot be trusted to enforce it. rds.force_ssl makes the SERVER refuse a
# non-TLS connection outright, which is what actually closes the gap; the
# DSNs in secrets.tf additionally pass sslmode=require so a well-behaved
# client never attempts plaintext in the first place. Family must track
# db_engine_version's major version (16.x here) — RDS parameter groups are
# versioned by major version, not by the exact minor this stack pins.
resource "aws_db_parameter_group" "this" {
  name_prefix = "${var.name_prefix}-pg16-"
  family      = "postgres16"

  parameter {
    name         = "rds.force_ssl"
    value        = "1"
    apply_method = "pending-reboot"
  }

  lifecycle { create_before_destroy = true }
}

resource "aws_db_instance" "this" {
  identifier     = "${var.name_prefix}-db"
  engine         = "postgres"
  engine_version = var.db_engine_version
  instance_class = var.db_instance_class

  allocated_storage     = var.db_allocated_storage_gb
  max_allocated_storage = var.db_allocated_storage_gb * 4
  storage_type          = "gp3"
  storage_encrypted     = true
  kms_key_id            = aws_kms_key.data.arn

  db_name  = "margince"
  username = "dbadmin"
  password = random_password.rds_master.result
  port     = 5432

  parameter_group_name   = aws_db_parameter_group.this.name
  db_subnet_group_name   = aws_db_subnet_group.this.name
  vpc_security_group_ids = [aws_security_group.db.id]
  publicly_accessible    = false

  multi_az                = var.db_multi_az
  backup_retention_period = var.db_backup_retention_days
  backup_window           = "03:00-04:00"
  maintenance_window      = "mon:04:30-mon:05:30"

  deletion_protection       = true
  skip_final_snapshot       = false
  final_snapshot_identifier = "${var.name_prefix}-db-final"

  # scripts/deploy/db-bootstrap.sql runs once, by hand, against this instance
  # as "dbadmin" (see deploy/terraform/aws/README.md) — it creates
  # margince_owner/margince_app as non-superuser roles inside the "margince"
  # database this instance already provisions, and grants margince_app the
  # table access the migration that runs first at boot expects it to already
  # hold.
}
