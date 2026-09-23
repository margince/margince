output "aws_region" {
  value = var.aws_region
}

output "instance_id" {
  value = aws_instance.this.id
}

output "instance_public_ip" {
  description = "The Elastic IP — point public_base_url's DNS record here (an A record, not a CNAME, since this is an IP not a hostname)."
  value       = aws_eip.this.public_ip
}

output "ssm_connect_command" {
  description = "Shell access — no SSH ingress on this instance's security group (network.tf), by design."
  value       = "aws ssm start-session --target ${aws_instance.this.id} --region ${var.aws_region}"
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

output "elasticache_endpoint" {
  value = aws_elasticache_replication_group.this.primary_endpoint_address
}

output "s3_blobstore_bucket" {
  value = aws_s3_bucket.blobstore.bucket
}

output "alerts_topic_arn" {
  description = "Empty when var.enable_deep_monitoring is false — there is no topic to subscribe to."
  value       = var.enable_deep_monitoring ? aws_sns_topic.alerts[0].arn : ""
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
