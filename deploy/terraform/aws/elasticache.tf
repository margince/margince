resource "random_password" "redis_auth" {
  length  = 32
  special = false
}

resource "aws_elasticache_replication_group" "this" {
  replication_group_id = "${var.name_prefix}-redis"
  description          = "Margince event bus / outbox relay"

  engine         = "redis"
  engine_version = "7.1"
  node_type      = var.redis_node_type
  port           = 6379

  num_cache_clusters         = 2
  automatic_failover_enabled = true
  multi_az_enabled           = true

  subnet_group_name  = aws_elasticache_subnet_group.this.name
  security_group_ids = [aws_security_group.redis.id]

  at_rest_encryption_enabled = true
  kms_key_id                 = aws_kms_key.data.arn
  transit_encryption_enabled = true
  # "preferred", not the default "required": the product's own Redis client
  # (backend/internal/platform/events/relay.go, ClientOptions) never sets
  # TLSConfig on the go-redis client, at all — grepped fresh in this session,
  # not assumed. "required" would refuse every connection the api and worker
  # actually make, since neither ever attempts TLS. "preferred" keeps
  # encryption available to any client that DOES negotiate it while still
  # admitting the plaintext connections this app's client is the only kind
  # it can open. The real fix is in the Go client, not here — this is the
  # honest floor until that lands, not a claim that the wire is protected.
  transit_encryption_mode = "preferred"
  auth_token              = random_password.redis_auth.result

  auto_minor_version_upgrade = true
  apply_immediately          = false
}
