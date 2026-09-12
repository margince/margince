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

# A fixed final_snapshot_identifier collides on a second deletion: RDS keeps
# the snapshot the first delete created, and DBSnapshotAlreadyExists refuses
# the next one that reuses the name. This suffix is created once and stays
# in state for the life of the instance, so it does not solve every case —
# an instance destroyed and recreated in the SAME state carries the SAME
# suffix, and a snapshot surviving from its first deletion still collides.
# It does solve the ordinary case (a fresh working directory / a fresh
# state), which a bare name solves none of.
resource "random_id" "final_snapshot" {
  byte_length = 4
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

  # The instance's own enabled_cloudwatch_logs_exports (below) only ships
  # whatever postgresql.log already contains — none of these are on by
  # default, so without them that export is a stream of nothing. All four
  # apply without a reboot (Postgres' own log_* GUCs are session/SIGHUP-level,
  # not the server-wide ones that need a restart like rds.force_ssl above).
  # 1000ms: log the tail of slow queries without logging every request.
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

# Enhanced Monitoring's OS-level metrics (CPU steal, swap, per-process) are
# collected by an agent RDS runs on your behalf, on a schedule this role's
# trust lets rds.amazonaws.com act under — Performance Insights' query-level
# view (below) tells you WHAT is slow, this tells you whether the underlying
# instance itself is the bottleneck. AWS's own managed policy is the
# documented grant for this, and nothing here is scoped narrower than AWS
# ships it: the role only trusts monitoring.rds.amazonaws.com, so there's no
# broader principal to narrow.
data "aws_iam_policy_document" "rds_enhanced_monitoring_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["monitoring.rds.amazonaws.com"]
    }
  }
}

resource "aws_iam_role" "rds_enhanced_monitoring" {
  name               = "${var.name_prefix}-rds-enhanced-monitoring"
  assume_role_policy = data.aws_iam_policy_document.rds_enhanced_monitoring_assume.json
  tags               = { Name = "${var.name_prefix}-rds-enhanced-monitoring", Component = "security" }
}

resource "aws_iam_role_policy_attachment" "rds_enhanced_monitoring" {
  role       = aws_iam_role.rds_enhanced_monitoring.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonRDSEnhancedMonitoringRole"
}

# RDS creates this log group itself, lazily, the first time
# enabled_cloudwatch_logs_exports below actually flushes a log line — with no
# retention policy, i.e. kept forever at growing cost, unless something else
# owns the group first. Declaring it here means Terraform's own
# log_retention_days governs it like every other log group in this stack
# (iam.tf), not whatever RDS's own default turns out to be. Name is fixed by
# RDS's own convention (/aws/rds/instance/<identifier>/<export>) — not ours
# to choose.
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

  # Pinned rather than left at whatever the account default happens to be —
  # RDS has rotated the account default CA before (rds-ca-2019 -> this one)
  # and will again; secrets.tf's sslmode=verify-full checks the server's cert
  # against the CA bundle README.md step 4 downloads once, by hand. An
  # unpinned instance that silently rolled onto a newer default CA than that
  # downloaded bundle recognizes would fail verify-full outright — pinning
  # here makes the CA an explicit, deliberate upgrade instead of a surprise.
  ca_cert_identifier = "rds-ca-rsa2048-g1"

  multi_az                = var.db_multi_az
  backup_retention_period = var.db_backup_retention_days
  backup_window           = "03:00-04:00"
  maintenance_window      = "mon:04:30-mon:05:30"
  # Tags (including Component/Environment, versions.tf's default_tags) follow
  # onto every automated and final snapshot — without this a snapshot is
  # untagged and invisible to a cost report or an automation script filtering
  # by tag, even though it bills like any other RDS storage.
  copy_tags_to_snapshot = true

  # Query-level visibility (which statements are slow) — retained 7 days,
  # the free tier ceiling; a longer window bills per vCPU. KMS'd under this
  # stack's own CMK like everything else at rest.
  performance_insights_enabled          = true
  performance_insights_kms_key_id       = aws_kms_key.data.arn
  performance_insights_retention_period = 7

  # Instance-level visibility (is the box itself the bottleneck) — 60s is
  # the coarsest interval that still resolves a transient CPU/IOPS spike;
  # requires the role above since RDS, not this account, publishes these
  # metrics.
  monitoring_interval = 60
  monitoring_role_arn = aws_iam_role.rds_enhanced_monitoring.arn

  # postgresql.log is the only place a connection failure, a deadlock, or a
  # slow-query entry (once log_min_duration_statement is set) shows up —
  # without this export it never leaves the instance at all.
  enabled_cloudwatch_logs_exports = ["postgresql"]

  deletion_protection       = true
  skip_final_snapshot       = false
  final_snapshot_identifier = "${var.name_prefix}-db-final-${random_id.final_snapshot.hex}"

  tags = { Name = "${var.name_prefix}-db", Component = "database" }

  # scripts/deploy/db-bootstrap.sql runs once, by hand, against this instance
  # as "dbadmin" (see deploy/terraform/aws/README.md) — it creates
  # margince_owner/margince_app as non-superuser roles inside the "margince"
  # database this instance already provisions, and grants margince_app the
  # table access the migration that runs first at boot expects it to already
  # hold.
}
