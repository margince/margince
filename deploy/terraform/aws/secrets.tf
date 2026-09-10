# One Secrets Manager secret per credential, so each maps to exactly one ECS
# task-definition "secrets" entry (a single valueFrom) rather than requiring
# every consumer to parse a shared JSON blob. Every secret below is sealed
# under the stack's CMK (kms.tf), not the aws/secretsmanager default — the
# ECS execution role's KMS grant (iam.tf) is what makes fetching them still
# work under that key.

resource "random_id" "keyvault_root_key" {
  byte_length = 32
}

resource "random_id" "webhook_key" {
  byte_length = 32
}

resource "random_id" "connector_state_key" {
  byte_length = 32
}

locals {
  db_host = aws_db_instance.this.address
  db_port = aws_db_instance.this.port
  # sslmode=require: the server-side backstop is aws_db_parameter_group.this's
  # rds.force_ssl (rds.tf) — this is the client-side half, so a working
  # deployment never even attempts the plaintext connection force_ssl would
  # otherwise have to refuse.
  owner_dsn  = "postgres://margince_owner:${urlencode(random_password.margince_owner.result)}@${local.db_host}:${local.db_port}/margince?sslmode=require"
  app_dsn    = "postgres://margince_app:${urlencode(random_password.margince_app.result)}@${local.db_host}:${local.db_port}/margince?sslmode=require"
  redis_host = aws_elasticache_replication_group.this.primary_endpoint_address
}

resource "aws_secretsmanager_secret" "owner_dsn" {
  name       = "${var.name_prefix}/margince-owner-dsn"
  kms_key_id = aws_kms_key.data.arn
}
resource "aws_secretsmanager_secret_version" "owner_dsn" {
  secret_id     = aws_secretsmanager_secret.owner_dsn.id
  secret_string = local.owner_dsn
}

resource "aws_secretsmanager_secret" "app_dsn" {
  name       = "${var.name_prefix}/margince-dsn"
  kms_key_id = aws_kms_key.data.arn
}
resource "aws_secretsmanager_secret_version" "app_dsn" {
  secret_id     = aws_secretsmanager_secret.app_dsn.id
  secret_string = local.app_dsn
}

resource "aws_secretsmanager_secret" "redis_password" {
  name       = "${var.name_prefix}/margince-redis-password"
  kms_key_id = aws_kms_key.data.arn
}
resource "aws_secretsmanager_secret_version" "redis_password" {
  secret_id     = aws_secretsmanager_secret.redis_password.id
  secret_string = random_password.redis_auth.result
}

resource "aws_secretsmanager_secret" "keyvault_root_key" {
  name       = "${var.name_prefix}/margince-keyvault-root-key"
  kms_key_id = aws_kms_key.data.arn
}
resource "aws_secretsmanager_secret_version" "keyvault_root_key" {
  secret_id     = aws_secretsmanager_secret.keyvault_root_key.id
  secret_string = random_id.keyvault_root_key.b64_std
}

resource "aws_secretsmanager_secret" "webhook_key" {
  name       = "${var.name_prefix}/margince-webhook-key"
  kms_key_id = aws_kms_key.data.arn
}
resource "aws_secretsmanager_secret_version" "webhook_key" {
  secret_id     = aws_secretsmanager_secret.webhook_key.id
  secret_string = random_id.webhook_key.b64_std
}

resource "aws_secretsmanager_secret" "connector_state_key" {
  name       = "${var.name_prefix}/margince-connector-state-key"
  kms_key_id = aws_kms_key.data.arn
}
resource "aws_secretsmanager_secret_version" "connector_state_key" {
  secret_id     = aws_secretsmanager_secret.connector_state_key.id
  secret_string = random_id.connector_state_key.b64_std
}

resource "aws_secretsmanager_secret" "admin_password" {
  name       = "${var.name_prefix}/margince-admin-password"
  kms_key_id = aws_kms_key.data.arn
}
resource "aws_secretsmanager_secret_version" "admin_password" {
  secret_id     = aws_secretsmanager_secret.admin_password.id
  secret_string = var.admin_bootstrap_password
}

# Created unconditionally so the ECS task definitions always have a valueFrom
# to point at; an empty string is a valid MARGINCE_LICENSE (runs unlicensed —
# only a production role without MARGINCE_ENV set refuses to boot on that).
resource "aws_secretsmanager_secret" "license" {
  name       = "${var.name_prefix}/margince-license"
  kms_key_id = aws_kms_key.data.arn
}
resource "aws_secretsmanager_secret_version" "license" {
  secret_id     = aws_secretsmanager_secret.license.id
  secret_string = var.license_token
}

resource "aws_secretsmanager_secret" "blobstore_access_key" {
  name       = "${var.name_prefix}/margince-blobstore-access-key"
  kms_key_id = aws_kms_key.data.arn
}
resource "aws_secretsmanager_secret_version" "blobstore_access_key" {
  secret_id     = aws_secretsmanager_secret.blobstore_access_key.id
  secret_string = aws_iam_access_key.blobstore.id
}

resource "aws_secretsmanager_secret" "blobstore_secret_key" {
  name       = "${var.name_prefix}/margince-blobstore-secret-key"
  kms_key_id = aws_kms_key.data.arn
}
resource "aws_secretsmanager_secret_version" "blobstore_secret_key" {
  secret_id     = aws_secretsmanager_secret.blobstore_secret_key.id
  secret_string = aws_iam_access_key.blobstore.secret
}
