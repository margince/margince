# Mounts a read-only margince.yaml at /app/config on the api and worker tasks.
# Terraform provisions the mount; it does not write into it — see this stack's
# README for the one-time `cp` an operator runs against the access point.

resource "aws_efs_file_system" "config" {
  creation_token = "${var.name_prefix}-config"
  encrypted      = true
  kms_key_id     = aws_kms_key.data.arn

  tags = { Name = "${var.name_prefix}-config" }
}

resource "aws_efs_mount_target" "config" {
  count           = var.az_count
  file_system_id  = aws_efs_file_system.config.id
  subnet_id       = aws_subnet.private[count.index].id
  security_groups = [aws_security_group.efs.id]
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

  tags = { Name = "${var.name_prefix}-config" }
}
