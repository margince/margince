resource "random_password" "redis_auth" {
  length  = 32
  special = false
}

# The default eviction policy (allkeys-lru/volatile-lru) silently drops keys
# under memory pressure — wrong for a replication group described as an
# "event bus / outbox relay", where a dropped key is a lost event, not a
# recoverable cache miss. noeviction makes that failure loud (OOM errors on
# write) instead of quiet.
resource "aws_elasticache_parameter_group" "this" {
  name   = "${var.name_prefix}-redis"
  family = "redis7"

  parameter {
    name  = "maxmemory-policy"
    value = "noeviction"
  }
}

resource "aws_elasticache_replication_group" "this" {
  replication_group_id = "${var.name_prefix}-redis"
  description          = "Margince event bus / outbox relay"

  engine               = "redis"
  engine_version       = "7.1"
  node_type            = var.redis_node_type
  port                 = 6379
  parameter_group_name = aws_elasticache_parameter_group.this.name

  num_cache_clusters         = 2
  automatic_failover_enabled = true
  multi_az_enabled           = true

  # Same retention as RDS (rds.tf) for the same reason: this data matters
  # enough to have a documented description ("outbox relay"), so it gets a
  # recovery point rather than none.
  snapshot_retention_limit = 7
  snapshot_window          = "03:00-04:00"

  subnet_group_name  = aws_elasticache_subnet_group.this.name
  security_group_ids = [aws_security_group.redis.id]

  at_rest_encryption_enabled = true
  kms_key_id                 = aws_kms_key.data.arn
  transit_encryption_enabled = true
  # "required", not "preferred": the product's own Redis client
  # (backend/internal/platform/events/relay.go) now takes a useTLS parameter
  # and negotiates TLS when MARGINCE_REDIS_TLS=true (ecs.tf's shared_env) —
  # both roles set it. Before that client change shipped, "preferred" was the
  # honest floor here: "required" would have refused every connection the api
  # and worker made, since neither ever attempted TLS. Deploy order still
  # matters — the api/worker images with TLS support must roll out before (or
  # in the same release as) this flip, never after.
  transit_encryption_mode = "required"
  auth_token              = random_password.redis_auth.result

  auto_minor_version_upgrade = true
  apply_immediately          = false
}
