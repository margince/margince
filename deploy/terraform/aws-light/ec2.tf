# The one thing this whole stack is built around: a single EC2 instance
# running api, worker, web, and an nginx reverse proxy as docker compose
# services (templates/user_data.sh.tpl). No ASG, no launch template — a
# replacement means re-running `terraform apply` (or, for an in-place image
# update, `terraform taint aws_instance.this` and applying) rather than
# traffic shifting to a healthy peer, because there is no peer. See this
# stack's README for what that tradeoff costs.

data "aws_ssm_parameter" "al2023_ami" {
  name = "/aws/service/ami-amazon-linux-latest/al2023-ami-kernel-default-${var.cpu_architecture}"
}

locals {
  ecr_registry = "${data.aws_caller_identity.current.account_id}.dkr.ecr.${var.aws_region}.amazonaws.com"

  # var.public_base_url is a full URL (scheme + host); nginx's server_name
  # and certbot's -d flag both need just the host. Fails at plan time on a
  # malformed URL rather than silently issuing a cert for the wrong name.
  domain = regex("^https?://([^/:]+)", var.public_base_url)[0]

  # Shared between both branches of nginx.conf.tftpl's %{ if enable_tls }
  # so the TLS and plain-HTTP variants route identically — see that file's
  # own header comment for why this isn't just duplicated in each branch.
  nginx_routing_block = <<-ROUTES
    client_max_body_size 32m;

    location = /healthz  { proxy_pass http://api:8080; }
    location = /readyz   { proxy_pass http://api:8080; }
    location = /metrics  { proxy_pass http://api:8080; }

    location /v1 { proxy_pass http://api:8080; }

    location = /webhooks/gmail   { proxy_pass http://api:8080; }
    location = /webhooks/graph   { proxy_pass http://api:8080; }
    location = /webhooks/hubspot { proxy_pass http://api:8080; }

    location /oauth/ { proxy_pass http://api:8080; }
    location /mcp     { proxy_pass http://api:8080; }
    location = /.well-known/oauth-authorization-server { proxy_pass http://api:8080; }
    # Prefix, not exact: also covers /.well-known/oauth-protected-resource/mcp
    location /.well-known/oauth-protected-resource { proxy_pass http://api:8080; }

    location / { proxy_pass http://web:8080; }

    proxy_set_header Host              $host;
    proxy_set_header X-Real-IP         $remote_addr;
    proxy_set_header X-Forwarded-For   $proxy_add_x_forwarded_for;
    proxy_set_header X-Forwarded-Proto $scheme;
  ROUTES

  nginx_conf = templatefile("${path.module}/templates/nginx.conf.tftpl", {
    enable_tls    = var.enable_tls
    domain        = local.domain
    routing_block = local.nginx_routing_block
  })

  # One entry per Secrets Manager secret this stack creates, mirroring the
  # full stack's shared_secrets list (deploy/terraform/aws/ecs.tf) —
  # user_data.sh.tpl fetches each of these into /opt/margince/.env at boot.
  secrets = [
    { env_name = "MARGINCE_OWNER_DSN", secret_id = aws_secretsmanager_secret.owner_dsn.name },
    { env_name = "MARGINCE_DSN", secret_id = aws_secretsmanager_secret.app_dsn.name },
    { env_name = "MARGINCE_REDIS_PASSWORD", secret_id = aws_secretsmanager_secret.redis_password.name },
    { env_name = "MARGINCE_KEYVAULT_ROOT_KEY", secret_id = aws_secretsmanager_secret.keyvault_root_key.name },
    { env_name = "MARGINCE_WEBHOOK_KEY", secret_id = aws_secretsmanager_secret.webhook_key.name },
    { env_name = "MARGINCE_CONNECTOR_STATE_KEY", secret_id = aws_secretsmanager_secret.connector_state_key.name },
    { env_name = "MARGINCE_ADMIN_PASSWORD", secret_id = aws_secretsmanager_secret.admin_password.name },
    { env_name = "MARGINCE_LICENSE", secret_id = aws_secretsmanager_secret.license.name },
    { env_name = "MARGINCE_BLOBSTORE_ACCESS_KEY", secret_id = aws_secretsmanager_secret.blobstore_access_key.name },
    { env_name = "MARGINCE_BLOBSTORE_SECRET_KEY", secret_id = aws_secretsmanager_secret.blobstore_secret_key.name },
  ]

  user_data = templatefile("${path.module}/templates/user_data.sh.tpl", {
    aws_region        = var.aws_region
    ecr_registry      = local.ecr_registry
    ecr_api_url       = aws_ecr_repository.api.repository_url
    ecr_worker_url    = aws_ecr_repository.worker.repository_url
    ecr_web_url       = aws_ecr_repository.web.repository_url
    image_tag         = var.image_tag
    blobstore_bucket  = aws_s3_bucket.blobstore.bucket
    redis_host        = local.redis_host
    public_base_url   = var.public_base_url
    api_log_group     = aws_cloudwatch_log_group.api.name
    worker_log_group  = aws_cloudwatch_log_group.worker.name
    web_log_group     = aws_cloudwatch_log_group.web.name
    secrets           = local.secrets
    nginx_conf_b64    = base64encode(local.nginx_conf)
    enable_tls        = var.enable_tls
    letsencrypt_email = var.letsencrypt_email
    domain            = local.domain
  })
}

resource "aws_eip" "this" {
  domain = "vpc"
  tags   = { Name = "${var.name_prefix}-ec2", Component = "network" }
}

resource "aws_eip_association" "this" {
  instance_id   = aws_instance.this.id
  allocation_id = aws_eip.this.id
}

resource "aws_instance" "this" {
  ami                    = data.aws_ssm_parameter.al2023_ami.value
  instance_type          = var.instance_type
  subnet_id              = aws_subnet.public.id
  vpc_security_group_ids = [aws_security_group.ec2.id]
  iam_instance_profile   = aws_iam_instance_profile.instance.name

  # IMDSv2 only — the instance role's own credentials (which can reach
  # Secrets Manager, ECR, and S3) are the single most valuable thing an
  # SSRF-shaped bug on this box could steal via IMDSv1's no-token GET.
  metadata_options {
    http_tokens   = "required"
    http_endpoint = "enabled"
  }

  root_block_device {
    volume_type = "gp3"
    volume_size = var.root_volume_gb
    encrypted   = true
  }

  user_data                   = local.user_data
  user_data_replace_on_change = true

  tags = { Name = "${var.name_prefix}-ec2", Component = "compute" }
}
