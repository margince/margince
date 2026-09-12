# Mounts a read-only margince.yaml at /app/config on the api and worker tasks.
# Terraform provisions the mount; it does not write into it — see this stack's
# README for the one-time `cp` an operator runs against the access point.

resource "aws_efs_file_system" "config" {
  creation_token = "${var.name_prefix}-config"
  encrypted      = true
  kms_key_id     = aws_kms_key.data.arn

  tags = { Name = "${var.name_prefix}-config", Component = "storage" }

  # EFS has no AWS-native deletion_protection the way RDS does (rds.tf) — this
  # is the Terraform-level guard against an accidental `terraform destroy`
  # wiping the operator-provisioned margince.yaml (README step 4). The backup
  # policy above covers recovery either way; this stops the accident before
  # it needs recovering from.
  lifecycle {
    prevent_destroy = true
  }
}

# AWS Backup coverage for this file system — new EFS file systems default to
# no backup plan, which would make the operator-provisioned margince.yaml
# (this stack's README documents the one-time `cp` onto the access point)
# unrecoverable from anything but redoing that step by hand. Same recovery
# reasoning RDS/ElastiCache already get (rds.tf's backup_retention_period,
# elasticache.tf's snapshot_retention_limit) — daily automatic backups via
# the account's default AWS Backup plan.
resource "aws_efs_backup_policy" "config" {
  file_system_id = aws_efs_file_system.config.id
  backup_policy {
    status = "ENABLED"
  }
}

resource "aws_efs_mount_target" "config" {
  count           = var.az_count
  file_system_id  = aws_efs_file_system.config.id
  subnet_id       = aws_subnet.private[count.index].id
  security_groups = [aws_security_group.efs.id]
}

# ecs.tf's task volumes already request transit_encryption = ENABLED
# client-side; this is the server-side backstop, mirroring s3.tf's
# DenyInsecureTransport — the mount itself refuses a non-TLS client
# regardless of what the task definition asks for.
resource "aws_efs_file_system_policy" "config" {
  file_system_id = aws_efs_file_system.config.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid       = "DenyInsecureTransport"
        Effect    = "Deny"
        Principal = "*"
        Action    = "elasticfilesystem:*"
        Resource  = aws_efs_file_system.config.arn
        Condition = {
          Bool = { "aws:SecureTransport" = "false" }
        }
      },
    ]
  })
}

resource "aws_efs_access_point" "config" {
  file_system_id = aws_efs_file_system.config.id

  posix_user {
    uid = 1000
    gid = 1000
  }

  root_directory {
    path = "/margince-config"
    creation_info {
      owner_uid   = 1000
      owner_gid   = 1000
      permissions = "0755"
    }
  }

  tags = { Name = "${var.name_prefix}-config", Component = "storage" }
}
