data "aws_caller_identity" "current" {}

resource "aws_cloudwatch_log_group" "api" {
  name              = "/ecs/${var.name_prefix}/api"
  retention_in_days = var.log_retention_days
  tags              = { Name = "${var.name_prefix}-api-logs", Component = "observability" }
}

resource "aws_cloudwatch_log_group" "worker" {
  name              = "/ecs/${var.name_prefix}/worker"
  retention_in_days = var.log_retention_days
  tags              = { Name = "${var.name_prefix}-worker-logs", Component = "observability" }
}

resource "aws_cloudwatch_log_group" "web" {
  name              = "/ecs/${var.name_prefix}/web"
  retention_in_days = var.log_retention_days
  tags              = { Name = "${var.name_prefix}-web-logs", Component = "observability" }
}

# Confused-deputy protection (docs.aws.amazon.com/AmazonECS/latest/developerguide/task-iam-roles.html,
# "we recommend that you use the aws:SourceAccount or aws:SourceArn condition
# keys"): without this, any AWS account's ECS control plane that references
# one of these role ARNs in a task definition it registers can assume it —
# the trust below only names the ecs-tasks.amazonaws.com service principal,
# not which account's tasks. aws:SourceArn can't be scoped to one cluster for
# this service (AWS's own doc: "specifying a specific cluster is not
# currently supported"), so the wildcard is the tightest available match, not
# a shortcut taken here.
data "aws_iam_policy_document" "ecs_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
    condition {
      test     = "StringEquals"
      variable = "aws:SourceAccount"
      values   = [data.aws_caller_identity.current.account_id]
    }
    condition {
      test     = "ArnLike"
      variable = "aws:SourceArn"
      values   = ["arn:aws:ecs:${var.aws_region}:${data.aws_caller_identity.current.account_id}:*"]
    }
  }
}

# ---- Execution role: pulls images, reads secrets on the task's behalf --------
# Shared by api and worker only — see execution_web below for why web gets
# its own, narrower role instead of this one.

resource "aws_iam_role" "execution" {
  name               = "${var.name_prefix}-ecs-execution"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume.json
  tags               = { Name = "${var.name_prefix}-ecs-execution", Component = "security" }
}

# No AmazonECSTaskExecutionRolePolicy attachment. That managed policy grants
# ecr:BatchGetImage/GetDownloadUrlForLayer/BatchCheckLayerAvailability and
# logs:CreateLogStream/PutLogEvents with Resource="*" — attaching it
# alongside the scoped statements below would not narrow anything, since IAM
# is additive-allow: the broadest grant for an action wins regardless of how
# tightly a sibling statement names its resources. Every action the managed
# policy would have granted is granted here instead, scoped to exactly this
# stack's own repos and log groups.
data "aws_iam_policy_document" "execution_extra" {
  statement {
    sid     = "ReadOwnSecrets"
    actions = ["secretsmanager:GetSecretValue"]
    resources = [
      aws_secretsmanager_secret.owner_dsn.arn,
      aws_secretsmanager_secret.app_dsn.arn,
      aws_secretsmanager_secret.redis_password.arn,
      aws_secretsmanager_secret.keyvault_root_key.arn,
      aws_secretsmanager_secret.webhook_key.arn,
      aws_secretsmanager_secret.connector_state_key.arn,
      aws_secretsmanager_secret.admin_password.arn,
      aws_secretsmanager_secret.license.arn,
      aws_secretsmanager_secret.blobstore_access_key.arn,
      aws_secretsmanager_secret.blobstore_secret_key.arn,
    ]
  }

  statement {
    sid     = "PullOwnImages"
    actions = ["ecr:GetDownloadUrlForLayer", "ecr:BatchGetImage", "ecr:BatchCheckLayerAvailability"]
    resources = [
      aws_ecr_repository.api.arn,
      aws_ecr_repository.worker.arn,
    ]
  }

  # ecr:GetAuthorizationToken cannot be scoped to a repository ARN — ECR
  # requires Resource="*" for this one action, unlike every pull action
  # above it (aws-iam skill, ecr.md: "it cannot be scoped to a repository").
  statement {
    sid       = "EcrAuth"
    actions   = ["ecr:GetAuthorizationToken"]
    resources = ["*"]
  }

  # Scoped to exactly the two log groups this role's task definitions write
  # to, per the aws-iam skill's own guidance: CloudWatch Logs actions belong
  # on the specific log group ARN, never Resource="*".
  statement {
    sid     = "WriteOwnLogs"
    actions = ["logs:CreateLogStream", "logs:PutLogEvents"]
    resources = [
      "${aws_cloudwatch_log_group.api.arn}:*",
      "${aws_cloudwatch_log_group.worker.arn}:*",
    ]
  }

  # Every secret this role reads and every image it pulls is sealed under
  # the stack's CMK (kms.tf) rather than an AWS-managed key — SSE at rest is
  # a promise the caller must be able to keep, and this is the permission
  # that lets it. DescribeKey is what Secrets Manager and ECR call
  # internally to validate the key before Decrypt; without it both fail
  # closed on a permissions error that names the key, not the secret.
  statement {
    sid       = "UseDataKey"
    actions   = ["kms:Decrypt", "kms:DescribeKey"]
    resources = [aws_kms_key.data.arn]
  }
}

resource "aws_iam_role_policy" "execution_extra" {
  name   = "${var.name_prefix}-ecs-execution-extra"
  role   = aws_iam_role.execution.id
  policy = data.aws_iam_policy_document.execution_extra.json
}

# ---- Web's own execution role: no secrets, on purpose -----------------------
# aws_iam_role.execution above can read every secret this stack creates, and
# the web (SPA/nginx) task uses none of them — no DSN, no keyvault key,
# nothing. Sharing one execution role across all three task defs would give
# an execution role compromised via web (or a misconfigured task definition
# that started referencing it) a read path to every credential the api and
# worker actually need.
#
# No AmazonECSTaskExecutionRolePolicy attachment here either, for exactly the
# reason aws_iam_role.execution's own comment gives: that managed policy's
# ECR/logs actions carry Resource="*", which would hand this role pull access
# to every ECR repo and write access to every log group in the account —
# strictly broader than the two things web's own task definition actually
# does (pull its own image, write its own log stream). Scoped statements
# below grant exactly that instead.

resource "aws_iam_role" "execution_web" {
  name               = "${var.name_prefix}-ecs-execution-web"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume.json
  tags               = { Name = "${var.name_prefix}-ecs-execution-web", Component = "security" }
}

data "aws_iam_policy_document" "execution_web_extra" {
  statement {
    sid     = "PullOwnImage"
    actions = ["ecr:GetDownloadUrlForLayer", "ecr:BatchGetImage", "ecr:BatchCheckLayerAvailability"]
    resources = [
      aws_ecr_repository.web.arn,
    ]
  }

  # Same reasoning as execution_extra's own EcrAuth statement: this action
  # cannot be scoped to a repository ARN, full stop, regardless of which
  # role requests it.
  statement {
    sid       = "EcrAuth"
    actions   = ["ecr:GetAuthorizationToken"]
    resources = ["*"]
  }

  statement {
    sid     = "WriteOwnLogs"
    actions = ["logs:CreateLogStream", "logs:PutLogEvents"]
    resources = [
      "${aws_cloudwatch_log_group.web.arn}:*",
    ]
  }

  # web's ECR repo is KMS-encrypted too (ecs.tf) — this is the narrow grant
  # that lets THIS role decrypt only that image, not the secrets the other
  # execution role can read.
  statement {
    sid       = "UseDataKeyForOwnImage"
    actions   = ["kms:Decrypt", "kms:DescribeKey"]
    resources = [aws_kms_key.data.arn]
  }
}

resource "aws_iam_role_policy" "execution_web_extra" {
  name   = "${var.name_prefix}-ecs-execution-web-extra"
  role   = aws_iam_role.execution_web.id
  policy = data.aws_iam_policy_document.execution_web_extra.json
}

# ---- Task roles: what the RUNNING container may call on its own behalf ------
# One per service, not shared, for the same isolation reason execution_web
# gets its own role above: api and worker mount the EFS config volume and so
# need the ClientMount grant below, web mounts nothing and gets none of it —
# sharing a single task role across all three would give web (the one
# service with no legitimate reason to touch the EFS filesystem) the same
# EFS access as api/worker the moment a future change added it there, with
# nothing in ecs.tf's task definitions to catch the leak. The blobstore
# client authenticates with static keys (see s3.tf), never a task role's
# credentials, so none of these carry blobstore permissions.

resource "aws_iam_role" "task_api" {
  name               = "${var.name_prefix}-ecs-task-api"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume.json
  tags               = { Name = "${var.name_prefix}-ecs-task-api", Component = "security" }
}

resource "aws_iam_role" "task_worker" {
  name               = "${var.name_prefix}-ecs-task-worker"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume.json
  tags               = { Name = "${var.name_prefix}-ecs-task-worker", Component = "security" }
}

# Empty, deliberately — web mounts no EFS volume and calls no other AWS API
# on its own behalf. An empty role is the honest floor, not a placeholder for
# "add something eventually".
resource "aws_iam_role" "task_web" {
  name               = "${var.name_prefix}-ecs-task-web"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume.json
  tags               = { Name = "${var.name_prefix}-ecs-task-web", Component = "security" }
}

# ecs.tf's task definitions mount the EFS config volume with
# authorization_config.iam = "ENABLED" — that setting means the mount is
# authorized by IAM alone, and EFS's own IAM model denies by default unless
# an explicit Allow exists in the file system's resource policy or the
# caller's identity policy (efs.tf's aws_efs_file_system_policy.config carries
# only a Deny, on purpose, so this is the Allow half). Without this grant,
# every api/worker task's ClientMount call fails closed and the container
# never boots — this is not defense in depth, it is the only Allow either
# task role has for this action.
data "aws_iam_policy_document" "task_efs_mount" {
  statement {
    sid     = "MountConfigVolume"
    actions = ["elasticfilesystem:ClientMount"]
    resources = [
      aws_efs_file_system.config.arn,
    ]
    condition {
      test     = "StringEquals"
      variable = "elasticfilesystem:AccessPointArn"
      values   = [aws_efs_access_point.config.arn]
    }
  }
}

resource "aws_iam_role_policy" "task_api_efs_mount" {
  name   = "${var.name_prefix}-ecs-task-api-efs-mount"
  role   = aws_iam_role.task_api.id
  policy = data.aws_iam_policy_document.task_efs_mount.json
}

resource "aws_iam_role_policy" "task_worker_efs_mount" {
  name   = "${var.name_prefix}-ecs-task-worker-efs-mount"
  role   = aws_iam_role.task_worker.id
  policy = data.aws_iam_policy_document.task_efs_mount.json
}
