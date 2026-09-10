resource "aws_cloudwatch_log_group" "api" {
  name              = "/ecs/${var.name_prefix}/api"
  retention_in_days = var.log_retention_days
}

resource "aws_cloudwatch_log_group" "worker" {
  name              = "/ecs/${var.name_prefix}/worker"
  retention_in_days = var.log_retention_days
}

resource "aws_cloudwatch_log_group" "web" {
  name              = "/ecs/${var.name_prefix}/web"
  retention_in_days = var.log_retention_days
}

data "aws_iam_policy_document" "ecs_assume" {
  statement {
    actions = ["sts:AssumeRole"]
    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}

# ---- Execution role: pulls images, reads secrets on the task's behalf --------
# Shared by api and worker only — see execution_web below for why web gets
# its own, narrower role instead of this one.

resource "aws_iam_role" "execution" {
  name               = "${var.name_prefix}-ecs-execution"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume.json
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
# worker actually need. AmazonECSTaskExecutionRolePolicy alone (ECR pull +
# CloudWatch Logs) is everything web's task definition asks for — plus its
# own narrow grant to decrypt its own, separately KMS-encrypted image.

resource "aws_iam_role" "execution_web" {
  name               = "${var.name_prefix}-ecs-execution-web"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume.json
}

resource "aws_iam_role_policy_attachment" "execution_web_managed" {
  role       = aws_iam_role.execution_web.name
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

# web's ECR repo is KMS-encrypted too (ecs.tf) — this is the narrow grant
# that lets THIS role decrypt only that image, not the secrets the other
# execution role can read.
data "aws_iam_policy_document" "execution_web_kms" {
  statement {
    sid       = "UseDataKeyForOwnImage"
    actions   = ["kms:Decrypt", "kms:DescribeKey"]
    resources = [aws_kms_key.data.arn]
  }
}

resource "aws_iam_role_policy" "execution_web_kms" {
  name   = "${var.name_prefix}-ecs-execution-web-kms"
  role   = aws_iam_role.execution_web.id
  policy = data.aws_iam_policy_document.execution_web_kms.json
}

# ---- Task role: what the RUNNING container may call on its own behalf -------
# The blobstore client authenticates with static keys (see s3.tf), never this
# role's credentials, so this role stays empty unless a later capability needs
# one — an empty role is the honest floor, not a placeholder for "add
# something eventually".

resource "aws_iam_role" "task" {
  name               = "${var.name_prefix}-ecs-task"
  assume_role_policy = data.aws_iam_policy_document.ecs_assume.json
}
