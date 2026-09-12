# Every "redis" name in this file (resource addresses, the security group,
# the subnet group, secret names, the slow-log group) names the ROLE and wire
# PROTOCOL this stack talks — not literally the Redis OSS engine binary,
# which the replication group below no longer runs. Renaming all of it to
# "valkey" would be a cosmetic, stack-wide rename for no behavior change;
# every consumer (app config, IAM grants, this file's own cross-references)
# still correctly calls it "redis" because that's the protocol it speaks.
resource "random_password" "redis_auth" {
  length  = 32
  special = false
}

# Same reasoning as rds.tf's random_id.final_snapshot: a fixed
# final_snapshot_identifier collides on a second delete (ElastiCache keeps the
# first delete's snapshot under that name), so this is created once and
# reused for the life of the replication group rather than a bare name.
resource "random_id" "redis_final_snapshot" {
  byte_length = 4
}

# The default eviction policy (allkeys-lru/volatile-lru) silently drops keys
# under memory pressure — wrong for a replication group described as an
# "event bus / outbox relay", where a dropped key is a lost event, not a
# recoverable cache miss. noeviction makes that failure loud (OOM errors on
# write) instead of quiet.
resource "aws_elasticache_parameter_group" "this" {
  name   = "${var.name_prefix}-redis"
  family = "valkey8"

  parameter {
    name  = "maxmemory-policy"
    value = "noeviction"
  }

  tags = { Name = "${var.name_prefix}-redis", Component = "cache" }
}

# Unlike RDS's log exports (rds.tf), ElastiCache never creates this group on
# its own — the replication group's log_delivery_configuration below refuses
# to enable at all unless the destination already exists, so this has to be
# created first, not lazily.
resource "aws_cloudwatch_log_group" "redis_slow_log" {
  name              = "/aws/elasticache/${var.name_prefix}-redis/slow-log"
  retention_in_days = var.log_retention_days
  tags              = { Name = "${var.name_prefix}-redis-slow-log", Component = "observability" }
}

resource "aws_elasticache_replication_group" "this" {
  replication_group_id = "${var.name_prefix}-redis"
  description          = "Margince event bus / outbox relay"

  # Slow-log is this stack's one signal for "why did the outbox relay stall"
  # — a command that blocked past the slowlog-log-slower-than threshold is
  # exactly the failure mode that a noeviction/OOM event (this parameter
  # group's own maxmemory-policy) would otherwise show up as only after the
  # fact, in an application-side timeout.
  log_delivery_configuration {
    destination      = aws_cloudwatch_log_group.redis_slow_log.name
    destination_type = "cloudwatch-logs"
    log_format       = "json"
    log_type         = "slow-log"
  }

  # Valkey, not Redis OSS: nothing is deployed yet (a fresh create, not an
  # in-place engine conversion — the harder, less-supported path some open
  # terraform-provider-aws issues describe), the app's client speaks the wire
  # protocol generically (TLS + AUTH token, backend/internal/platform/events/relay.go),
  # and Redis OSS 7.1 is the last version ElastiCache will ever move forward
  # on a shared roadmap with — AWS's own new capability (vector search,
  # durability modes) lands on Valkey, not Redis OSS, from here on. Picking
  # Redis now would only mean paying this exact migration later, with live
  # data instead of none.
  engine               = "valkey"
  engine_version       = "8.2"
  node_type            = var.redis_node_type
  port                 = 6379
  parameter_group_name = aws_elasticache_parameter_group.this.name

  # alarms.tf's local.redis_node_count is the same 2, kept as one number
  # rather than two: that file derives its per-node alarm addressing from
  # this value's own naming convention, so the two must never disagree.
  num_cache_clusters         = local.redis_node_count
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

  # ElastiCache has no deletion_protection flag the way RDS does (rds.tf) —
  # this is the closest equivalent AWS gives it: a final snapshot on delete
  # rather than losing the outbox relay's data outright, and Terraform's own
  # prevent_destroy below as the guard against an accidental `terraform
  # destroy` (it does not stop a console/CLI delete the way RDS's own flag
  # does — there is no API-level equivalent for ElastiCache to reach for).
  final_snapshot_identifier = "${var.name_prefix}-redis-final-${random_id.redis_final_snapshot.hex}"

  tags = { Name = "${var.name_prefix}-redis", Component = "cache" }

  lifecycle {
    prevent_destroy = true
  }
}
