# One EC2 instance role — there's only one instance, and no execution-role/
# task-role split to make the way the full stack does (deploy/terraform/aws/iam.tf):
# api, worker, and web all run as containers on the SAME instance, under the
# SAME role, so splitting "pulls the image" from "runs the container" buys
# no isolation here the way it does across separate ECS tasks.

resource "aws_cloudwatch_log_group" "api" {
  name              = "/margince-light/api"
  retention_in_days = var.log_retention_days
  tags              = { Name = "${var.name_prefix}-api-logs", Component = "observability" }
}

resource "aws_cloudwatch_log_group" "worker" {
  name              = "/margince-light/worker"
  retention_in_days = var.log_retention_days
  tags              = { Name = "${var.name_prefix}-worker-logs", Component = "observability" }
}

resource "aws_cloudwatch_log_group" "web" {
  name              = "/margince-light/web"
  retention_in_days = var.log_retention_days
  tags              = { Name = "${var.name_prefix}-web-logs", Component = "observability" }
}

resource "aws_iam_role" "instance" {
  name = "${var.name_prefix}-ec2"
  # No aws:SourceAccount/SourceArn condition needed here the way the full
  # stack's ecs_assume carries — ec2.amazonaws.com's own AssumeRole is
  # already scoped to instances launched in THIS account (the trust
  # relationship is per-account by construction for EC2, unlike the shared
  # ecs-tasks.amazonaws.com principal ECS confused-deputy guidance warns
  # about).
  assume_role_policy = jsonencode({
    Version = "2012-10-17"
    Statement = [{
      Effect    = "Allow"
      Principal = { Service = "ec2.amazonaws.com" }
      Action    = "sts:AssumeRole"
    }]
  })
  tags = { Name = "${var.name_prefix}-ec2", Component = "security" }
}

resource "aws_iam_instance_profile" "instance" {
  name = "${var.name_prefix}-ec2"
  role = aws_iam_role.instance.name
  tags = { Name = "${var.name_prefix}-ec2", Component = "security" }
}

# SSM Session Manager instead of SSH — see network.tf's own note on why
# there is no port-22 ingress rule. This AWS-managed policy is the
# documented grant for it; nothing here narrows it further since it carries
# no destructive action (it's how an operator gets a shell, not how the app
# runs).
resource "aws_iam_role_policy_attachment" "ssm" {
  role       = aws_iam_role.instance.name
  policy_arn = "arn:aws:iam::aws:policy/AmazonSSMManagedInstanceCore"
}

data "aws_iam_policy_document" "instance_extra" {
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
      aws_ecr_repository.web.arn,
    ]
  }

  # Cannot be scoped to a repository ARN — same ECR API constraint the full
  # stack's own EcrAuth statement documents.
  statement {
    sid       = "EcrAuth"
    actions   = ["ecr:GetAuthorizationToken"]
    resources = ["*"]
  }

  # The one object this instance ever reads from the blobstore bucket on
  # its own behalf: the margince.yaml config object ec2.tf's user-data
  # fetches at boot. Everything else in that bucket (attachments) is
  # reached only via the blobstore IAM user's static keys (s3.tf), never
  # this role.
  statement {
    sid       = "ReadConfigObject"
    actions   = ["s3:GetObject"]
    resources = ["${aws_s3_bucket.blobstore.arn}/config/margince.yaml"]
  }

  statement {
    sid     = "WriteOwnLogs"
    actions = ["logs:CreateLogStream", "logs:PutLogEvents", "logs:DescribeLogStreams"]
    resources = [
      "${aws_cloudwatch_log_group.api.arn}:*",
      "${aws_cloudwatch_log_group.worker.arn}:*",
      "${aws_cloudwatch_log_group.web.arn}:*",
    ]
  }
}

resource "aws_iam_role_policy" "instance_extra" {
  name   = "${var.name_prefix}-ec2-extra"
  role   = aws_iam_role.instance.id
  policy = data.aws_iam_policy_document.instance_extra.json
}
