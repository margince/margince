output "alb_dns_name" {
  value = aws_lb.this.dns_name
}

output "ecs_cluster_name" {
  value = aws_ecs_cluster.this.name
}

output "ecr_api_repository_url" {
  value = aws_ecr_repository.api.repository_url
}

output "ecr_worker_repository_url" {
  value = aws_ecr_repository.worker.repository_url
}

output "ecr_web_repository_url" {
  value = aws_ecr_repository.web.repository_url
}

output "rds_endpoint" {
  value = aws_db_instance.this.address
}

output "elasticache_primary_endpoint" {
  value = aws_elasticache_replication_group.this.primary_endpoint_address
}

output "efs_file_system_id" {
  value = aws_efs_file_system.config.id
}

output "efs_config_access_point_id" {
  value = aws_efs_access_point.config.id
}

output "s3_blobstore_bucket" {
  value = aws_s3_bucket.blobstore.bucket
}

output "kms_key_arn" {
  value = aws_kms_key.data.arn
}

output "secret_arns" {
  description = "Secrets Manager ARNs (not values) for every credential this stack seals."
  value = {
    owner_dsn            = aws_secretsmanager_secret.owner_dsn.arn
    app_dsn              = aws_secretsmanager_secret.app_dsn.arn
    redis_password       = aws_secretsmanager_secret.redis_password.arn
    keyvault_root_key    = aws_secretsmanager_secret.keyvault_root_key.arn
    webhook_key          = aws_secretsmanager_secret.webhook_key.arn
    connector_state_key  = aws_secretsmanager_secret.connector_state_key.arn
    admin_password       = aws_secretsmanager_secret.admin_password.arn
    license              = aws_secretsmanager_secret.license.arn
    blobstore_access_key = aws_secretsmanager_secret.blobstore_access_key.arn
    blobstore_secret_key = aws_secretsmanager_secret.blobstore_secret_key.arn
  }
}
