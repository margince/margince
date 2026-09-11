# IMMUTABLE: a pushed tag can never be silently overwritten — the only way to
# ship a new image is a new tag, which is also what makes var.image_tag's "no
# floating latest" rule (variables.tf) actually enforceable rather than
# advisory. Each repo's own KMS encryption_configuration is what makes each
# execution role's own UseDataKey grant (iam.tf) meaningful — api/worker
# under the shared execution role's grant, web under execution_web's own.
resource "aws_ecr_repository" "api" {
  name                 = "${var.name_prefix}/api"
  image_tag_mutability = "IMMUTABLE"
  image_scanning_configuration { scan_on_push = true }
  encryption_configuration {
    encryption_type = "KMS"
    kms_key         = aws_kms_key.data.arn
  }
  tags = { Name = "${var.name_prefix}-api", Component = "container-registry" }
}

resource "aws_ecr_repository" "worker" {
  name                 = "${var.name_prefix}/worker"
  image_tag_mutability = "IMMUTABLE"
  image_scanning_configuration { scan_on_push = true }
  encryption_configuration {
    encryption_type = "KMS"
    kms_key         = aws_kms_key.data.arn
  }
  tags = { Name = "${var.name_prefix}-worker", Component = "container-registry" }
}

resource "aws_ecr_repository" "web" {
  name                 = "${var.name_prefix}/web"
  image_tag_mutability = "IMMUTABLE"
  image_scanning_configuration { scan_on_push = true }
  encryption_configuration {
    encryption_type = "KMS"
    kms_key         = aws_kms_key.data.arn
  }
  tags = { Name = "${var.name_prefix}-web", Component = "container-registry" }
}

# IMMUTABLE tags mean every push accumulates rather than overwrites — an
# untagged image (the previous digest, once a tag moves) is dead weight and
# attack surface (an unpatched image nobody references) with no reason to
# keep it. `sinceImagePulled` cannot pair with `expire` (it only drives
# `transition`, per ECR's own lifecycle semantics), so this is the direct
# `expire untagged after N days` rule rather than a pull-activity-based one.
#
# Untagged cleanup alone still leaves every TAGGED (released) image growing
# forever — IMMUTABLE means a tag is never reused, so nothing ever naturally
# frees one, unlike a mutable "latest"-style repo where a new push already
# reclaims the old digest's tag. Rule 2 is the tagged-image half of the same
# cleanup: keep the most recent var.ecr_tagged_image_retain_count releases
# (rollback material), expire the rest. tagPatternList = ["*"] matches every
# tag rather than naming release-version tags one at a time, since this
# stack's own var.image_tag is an open-ended release identifier, not a fixed
# set of prefixes to enumerate.
locals {
  untagged_expiry_policy = jsonencode({
    rules = [
      {
        rulePriority = 1
        description  = "Expire untagged images after 14 days"
        selection = {
          tagStatus   = "untagged"
          countType   = "sinceImagePushed"
          countUnit   = "days"
          countNumber = 14
        }
        action = { type = "expire" }
      },
      {
        rulePriority = 2
        description  = "Keep only the most recent ${var.ecr_tagged_image_retain_count} tagged (released) images"
        selection = {
          tagStatus      = "tagged"
          tagPatternList = ["*"]
          countType      = "imageCountMoreThan"
          countNumber    = var.ecr_tagged_image_retain_count
        }
        action = { type = "expire" }
      },
    ]
  })
}

resource "aws_ecr_lifecycle_policy" "api" {
  repository = aws_ecr_repository.api.name
  policy     = local.untagged_expiry_policy
}

resource "aws_ecr_lifecycle_policy" "worker" {
  repository = aws_ecr_repository.worker.name
  policy     = local.untagged_expiry_policy
}

resource "aws_ecr_lifecycle_policy" "web" {
  repository = aws_ecr_repository.web.name
  policy     = local.untagged_expiry_policy
}

# Registry-level, not repository-level, and scoped to only THIS stack's
# repos: aws_ecr_registry_scanning_configuration is a singleton per
# account+region — applying an unscoped rule here would silently start
# billing and scanning every OTHER repo in the account too, not just the
# three this stack owns. Enhanced scanning (continuous, Inspector-backed)
# supersedes each repo's own scan_on_push for any repo the filter below
# matches — it re-scans on every new CVE disclosure, not only at push time,
# which scan_on_push alone never catches for an image already sitting in the
# repo. This is a metered feature (Inspector charges per image scanned) on
# top of the basic scanning this stack shipped with — see deploy/terraform/aws/README.md.
resource "aws_ecr_registry_scanning_configuration" "this" {
  scan_type = "ENHANCED"

  rule {
    scan_frequency = "CONTINUOUS_SCAN"
    repository_filter {
      filter      = "${var.name_prefix}/*"
      filter_type = "WILDCARD"
    }
  }
}

resource "aws_ecs_cluster" "this" {
  name = "${var.name_prefix}-cluster"

  setting {
    name  = "containerInsights"
    value = "enabled"
  }

  tags = { Name = "${var.name_prefix}-cluster", Component = "compute" }
}

locals {
  blobstore_endpoint = "s3.${var.aws_region}.amazonaws.com"

  # Secrets Manager "secrets" entries every role-carrying task shares.
  shared_secrets = [
    { name = "MARGINCE_OWNER_DSN", valueFrom = aws_secretsmanager_secret.owner_dsn.arn },
    { name = "MARGINCE_DSN", valueFrom = aws_secretsmanager_secret.app_dsn.arn },
    { name = "MARGINCE_REDIS_PASSWORD", valueFrom = aws_secretsmanager_secret.redis_password.arn },
    { name = "MARGINCE_KEYVAULT_ROOT_KEY", valueFrom = aws_secretsmanager_secret.keyvault_root_key.arn },
    { name = "MARGINCE_WEBHOOK_KEY", valueFrom = aws_secretsmanager_secret.webhook_key.arn },
    { name = "MARGINCE_CONNECTOR_STATE_KEY", valueFrom = aws_secretsmanager_secret.connector_state_key.arn },
    { name = "MARGINCE_ADMIN_PASSWORD", valueFrom = aws_secretsmanager_secret.admin_password.arn },
    { name = "MARGINCE_LICENSE", valueFrom = aws_secretsmanager_secret.license.arn },
    { name = "MARGINCE_BLOBSTORE_ACCESS_KEY", valueFrom = aws_secretsmanager_secret.blobstore_access_key.arn },
    { name = "MARGINCE_BLOBSTORE_SECRET_KEY", valueFrom = aws_secretsmanager_secret.blobstore_secret_key.arn },
  ]

  shared_env = [
    { name = "MARGINCE_CONFIG", value = "/app/config/margince.yaml" },
    { name = "MARGINCE_REDIS", value = "${local.redis_host}:6379" },
    # elasticache.tf's transit_encryption_mode = "required" refuses a
    # plaintext connection outright — this is what makes the app's own
    # connection attempts negotiate TLS instead of failing to connect at all.
    { name = "MARGINCE_REDIS_TLS", value = "true" },
    { name = "MARGINCE_PUBLIC_BASE_URL", value = var.public_base_url },
    { name = "MARGINCE_BLOBSTORE_ENDPOINT", value = local.blobstore_endpoint },
    { name = "MARGINCE_BLOBSTORE_BUCKET", value = aws_s3_bucket.blobstore.bucket },
    { name = "MARGINCE_BLOBSTORE_REGION", value = var.aws_region },
    { name = "MARGINCE_BLOBSTORE_USE_SSL", value = "true" },
    # s3.tf's DenyWrongKMSKey statement refuses any write that doesn't carry
    # this exact key id — without this variable set, the client would send no
    # SSE header at all and every upload would be denied.
    { name = "MARGINCE_BLOBSTORE_KMS_KEY_ID", value = aws_kms_key.data.arn },
    { name = "MARGINCE_LOG_FORMAT", value = "json" },
  ]

  config_volume_name = "margince-config"
}

resource "aws_ecs_task_definition" "api" {
  family                   = "${var.name_prefix}-api"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = var.api_cpu
  memory                   = var.api_memory
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.task_api.arn

  runtime_platform {
    cpu_architecture        = var.cpu_architecture
    operating_system_family = "LINUX"
  }

  volume {
    name = local.config_volume_name
    efs_volume_configuration {
      file_system_id     = aws_efs_file_system.config.id
      transit_encryption = "ENABLED"
      authorization_config {
        access_point_id = aws_efs_access_point.config.id
        iam             = "ENABLED"
      }
    }
  }

  tags = { Name = "${var.name_prefix}-api", Component = "compute-api" }

  container_definitions = jsonencode([
    {
      name      = "api"
      image     = "${aws_ecr_repository.api.repository_url}:${var.image_tag}"
      essential = true
      # The image already runs as a non-root user (Dockerfile's `USER app`);
      # this drops every Linux capability the root user itself would have
      # had, on top of that — a statically-linked Go binary with no cgo needs
      # none of them, and Fargate's own restrictions (no privileged, no
      # capability additions beyond CAP_SYS_PTRACE) mean this can only narrow
      # further, never conflict with something the platform already grants.
      linuxParameters = { capabilities = { drop = ["ALL"] } }
      portMappings    = [{ containerPort = 8080, protocol = "tcp" }]
      # Fargate default is 30s; the api's own shutdown is graceful (it stops
      # its listener LAST, per docs/reference/configuration.md), so it is
      # worth more than the default to let in-flight requests actually drain
      # rather than being cut off mid-response.
      stopTimeout = 60
      environment = local.shared_env
      secrets     = local.shared_secrets
      mountPoints = [{
        sourceVolume  = local.config_volume_name
        containerPath = "/app/config"
        readOnly      = true
      }]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.api.name
          "awslogs-region"        = var.aws_region
          "awslogs-stream-prefix" = "api"
        }
      }
    }
  ])
}

resource "aws_ecs_task_definition" "worker" {
  family                   = "${var.name_prefix}-worker"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = var.worker_cpu
  memory                   = var.worker_memory
  execution_role_arn       = aws_iam_role.execution.arn
  task_role_arn            = aws_iam_role.task_worker.arn

  runtime_platform {
    cpu_architecture        = var.cpu_architecture
    operating_system_family = "LINUX"
  }

  volume {
    name = local.config_volume_name
    efs_volume_configuration {
      file_system_id     = aws_efs_file_system.config.id
      transit_encryption = "ENABLED"
      authorization_config {
        access_point_id = aws_efs_access_point.config.id
        iam             = "ENABLED"
      }
    }
  }

  tags = { Name = "${var.name_prefix}-worker", Component = "compute-worker" }

  container_definitions = jsonencode([
    {
      name      = "worker"
      image     = "${aws_ecr_repository.worker.repository_url}:${var.image_tag}"
      essential = true
      # Same reasoning as api's own linuxParameters.
      linuxParameters = { capabilities = { drop = ["ALL"] } }
      # Same reasoning as api's — graceful shutdown (in-flight subscriber
      # handlers finish their ack before exit, per configuration.md) is worth
      # more time than Fargate's 30s default.
      stopTimeout = 60
      environment = concat(local.shared_env, [
        { name = "MARGINCE_OBSERVE_ADDR", value = "0.0.0.0:9101" },
      ])
      secrets = local.shared_secrets
      mountPoints = [{
        sourceVolume  = local.config_volume_name
        containerPath = "/app/config"
        readOnly      = true
      }]
      # No container healthCheck: the worker image (alpine + ca-certificates +
      # tzdata only, see Dockerfile's `worker` stage) ships no curl/wget/nc to
      # probe :9101/healthz with, and ECS container health checks only run an
      # exec'd command — there is nothing in the image to exec. ECS still
      # tracks the task's RUNNING state; a deeper check needs either adding an
      # HTTP client to the image or an external prober hitting the task's ENI,
      # neither of which this stack adds on your behalf.
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.worker.name
          "awslogs-region"        = var.aws_region
          "awslogs-stream-prefix" = "worker"
        }
      }
    }
  ])
}

resource "aws_ecs_task_definition" "web" {
  family                   = "${var.name_prefix}-web"
  requires_compatibilities = ["FARGATE"]
  network_mode             = "awsvpc"
  cpu                      = var.web_cpu
  memory                   = var.web_memory
  # execution_web, not execution: web reads no secrets, so it gets no path to
  # any (see iam.tf).
  execution_role_arn = aws_iam_role.execution_web.arn
  task_role_arn      = aws_iam_role.task_web.arn

  runtime_platform {
    cpu_architecture        = var.cpu_architecture
    operating_system_family = "LINUX"
  }

  tags = { Name = "${var.name_prefix}-web", Component = "compute-web" }

  container_definitions = jsonencode([
    {
      name      = "web"
      image     = "${aws_ecr_repository.web.repository_url}:${var.image_tag}"
      essential = true
      # Same reasoning as api's own linuxParameters — nginx-unprivileged
      # (Dockerfile's `web` stage) already needs none of the capabilities
      # this drops: port 8080 is unprivileged, and the base image is built
      # to run without CAP_NET_BIND_SERVICE or any other addition.
      linuxParameters = { capabilities = { drop = ["ALL"] } }
      portMappings    = [{ containerPort = 8080, protocol = "tcp" }]
      logConfiguration = {
        logDriver = "awslogs"
        options = {
          "awslogs-group"         = aws_cloudwatch_log_group.web.name
          "awslogs-region"        = var.aws_region
          "awslogs-stream-prefix" = "web"
        }
      }
    }
  ])
}

resource "aws_ecs_service" "api" {
  name            = "${var.name_prefix}-api"
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.api.arn
  desired_count   = var.api_desired_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = aws_subnet.private[*].id
    security_groups  = [aws_security_group.ecs_tasks.id]
    assign_public_ip = false
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.api.arn
    container_name   = "api"
    container_port   = 8080
  }

  # A bad deploy without this sits at whatever health the ALB reports with no
  # automatic recovery — rollback is what turns a failed rollout back into a
  # working one without a human re-running apply. Grace period covers the
  # migrate-then-serve startup path (see the Dockerfile entrypoint) so a slow
  # first boot is not mistaken for a failed one mid-rollout.
  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }
  health_check_grace_period_seconds = 60

  tags = { Name = "${var.name_prefix}-api", Component = "compute-api" }

  depends_on = [aws_lb_listener.https]

  # desired_count is the FLOOR the appautoscaling_target below scales from,
  # not the steady-state value — without this, every apply would fight the
  # autoscaler back down to api_desired_count the moment it had scaled out.
  lifecycle {
    ignore_changes = [desired_count]
  }
}

resource "aws_ecs_service" "worker" {
  name            = "${var.name_prefix}-worker"
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.worker.arn
  desired_count   = var.worker_desired_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = aws_subnet.private[*].id
    security_groups  = [aws_security_group.ecs_tasks.id]
    assign_public_ip = false
  }

  # No load balancer on this service, so no health_check_grace_period_seconds —
  # rollback still protects against a worker that crash-loops on boot.
  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }

  tags = { Name = "${var.name_prefix}-worker", Component = "compute-worker" }

  # Same reasoning as aws_ecs_service.api's own ignore_changes.
  lifecycle {
    ignore_changes = [desired_count]
  }
}

# ---- Application Auto Scaling -------------------------------------------------
# desired_count above is each service's FLOOR, not its steady-state size —
# without a scaling policy it is also the ceiling, which means a traffic
# spike (api) or a queue backlog (worker) has nowhere to go but slower
# responses and growing lag. Target tracking on CPU rather than a custom
# metric: this stack has no queue-depth metric of its own to track yet, and
# CPU is the honest floor every workload here already emits for free.
resource "aws_appautoscaling_target" "api" {
  service_namespace  = "ecs"
  resource_id        = "service/${aws_ecs_cluster.this.name}/${aws_ecs_service.api.name}"
  scalable_dimension = "ecs:service:DesiredCount"
  min_capacity       = var.api_desired_count
  max_capacity       = var.api_autoscaling_max_count
}

resource "aws_appautoscaling_policy" "api_cpu" {
  name               = "${var.name_prefix}-api-cpu"
  policy_type        = "TargetTrackingScaling"
  service_namespace  = aws_appautoscaling_target.api.service_namespace
  resource_id        = aws_appautoscaling_target.api.resource_id
  scalable_dimension = aws_appautoscaling_target.api.scalable_dimension

  target_tracking_scaling_policy_configuration {
    predefined_metric_specification {
      predefined_metric_type = "ECSServiceAverageCPUUtilization"
    }
    target_value       = 70
    scale_in_cooldown  = 300
    scale_out_cooldown = 60
  }
}

resource "aws_appautoscaling_target" "worker" {
  service_namespace  = "ecs"
  resource_id        = "service/${aws_ecs_cluster.this.name}/${aws_ecs_service.worker.name}"
  scalable_dimension = "ecs:service:DesiredCount"
  min_capacity       = var.worker_desired_count
  max_capacity       = var.worker_autoscaling_max_count
}

resource "aws_appautoscaling_policy" "worker_cpu" {
  name               = "${var.name_prefix}-worker-cpu"
  policy_type        = "TargetTrackingScaling"
  service_namespace  = aws_appautoscaling_target.worker.service_namespace
  resource_id        = aws_appautoscaling_target.worker.resource_id
  scalable_dimension = aws_appautoscaling_target.worker.scalable_dimension

  target_tracking_scaling_policy_configuration {
    predefined_metric_specification {
      predefined_metric_type = "ECSServiceAverageCPUUtilization"
    }
    target_value      = 70
    scale_in_cooldown = 300
    # Worker backlog (event relay consumers) builds up faster than api
    # request queueing does under the same CPU pressure — scale out sooner.
    scale_out_cooldown = 30
  }
}

resource "aws_ecs_service" "web" {
  name            = "${var.name_prefix}-web"
  cluster         = aws_ecs_cluster.this.id
  task_definition = aws_ecs_task_definition.web.arn
  desired_count   = var.web_desired_count
  launch_type     = "FARGATE"

  network_configuration {
    subnets          = aws_subnet.private[*].id
    security_groups  = [aws_security_group.ecs_tasks.id]
    assign_public_ip = false
  }

  load_balancer {
    target_group_arn = aws_lb_target_group.web.arn
    container_name   = "web"
    container_port   = 8080
  }

  deployment_circuit_breaker {
    enable   = true
    rollback = true
  }
  health_check_grace_period_seconds = 60

  tags = { Name = "${var.name_prefix}-web", Component = "compute-web" }

  depends_on = [aws_lb_listener.https]
}
