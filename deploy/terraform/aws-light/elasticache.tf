# Same naming note as the full stack (deploy/terraform/aws/elasticache.tf):
# every "redis" name here (security group, subnet group, secret) names the
# protocol this stack speaks, not the Valkey engine binary underneath.

resource "random_password" "redis_auth" {
  length  = 32
  special = false
}

# The default eviction policy silently drops keys under memory pressure —
# wrong for an event bus / outbox relay, where a dropped key is a lost
# event. Same reasoning and same setting as the full stack.
resource "aws_elasticache_parameter_group" "this" {
  name   = "${var.name_prefix}-redis"
  family = "valkey8"

  parameter {
    name  = "maxmemory-policy"
    value = "noeviction"
  }

  tags = { Name = "${var.name_prefix}-redis", Component = "cache" }
}

# aws_elasticache_cluster (the standalone-node resource) cannot express
# at-rest encryption, transit encryption, or an AUTH token at all — in both
# the AWS API and this provider, those three are Replication Group
# properties only, never a Cache Cluster's. Keeping the transit-encryption
# and AUTH-token floor (this stack does NOT cut those — see rds.tf's own
# note on which floors "light" keeps) means using a replication group here
# too, just with num_cache_clusters = 1 and automatic_failover_enabled/
# multi_az_enabled = false: one node, no standby, no automatic failover —
# the actual "light" tradeoff — rather than the full stack's 2-node,
# automatic-failover replication group.
resource "aws_elasticache_replication_group" "this" {
  replication_group_id = "${var.name_prefix}-redis"
  description          = "Margince event bus / outbox relay (light: single node, no failover)"

  engine               = "valkey"
  engine_version       = "8.2"
  node_type            = var.redis_node_type
  port                 = 6379
  parameter_group_name = aws_elasticache_parameter_group.this.name

  num_cache_clusters         = 1
  automatic_failover_enabled = false
  multi_az_enabled           = false

  subnet_group_name  = aws_elasticache_subnet_group.this.name
  security_group_ids = [aws_security_group.redis.id]

  # AWS-managed key (no kms_key_id) — same "light" tradeoff as rds.tf,
  # storage is still encrypted, just not under a CMK this stack owns.
  at_rest_encryption_enabled = true
  transit_encryption_enabled = true
  transit_encryption_mode    = "required"
  auth_token                 = random_password.redis_auth.result

  auto_minor_version_upgrade = true
  apply_immediately          = false

  tags = { Name = "${var.name_prefix}-redis", Component = "cache" }

  # No final_snapshot_identifier, no prevent_destroy: a one-node cache with
  # no failover is already the "restore from durable stores, not from a
  # cache snapshot" tradeoff — see this file's note on recoverability. An
  # operator tearing down this light stack should be able to
  # `terraform destroy` cleanly.
}
