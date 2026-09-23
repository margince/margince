# Deploying on AWS — the light stack

[`deploy/terraform/aws/`](../deploy/terraform/aws/README.md) is the
production-shaped reference: ECS Fargate, an ALB with WAF, EFS, a
customer-managed KMS key. [`deploy/terraform/aws-light/`](../deploy/terraform/aws-light/README.md)
is the other shape — **one EC2 instance** running `api`, `worker`, and `web`
as docker compose services behind the instance's own nginx, with RDS and
ElastiCache still managed. No ALB, no ECS, no EFS, no customer-managed KMS,
no NAT gateway, no autoscaling, no multi-AZ compute.

It exists for a single audience: a small deployment, a demo, or a
cost-constrained installation that accepts one instance as a single point of
failure in plain terms. For anything that needs to survive an AZ outage, scale
past one box, or hold a CloudTrail key-usage trail over its own encryption,
[the full stack](../deploy/terraform/aws/README.md) already serves that better
— this one would not pay for its own maintenance as a second copy of it.

**Provisioning commands, variables, and the security posture live in
[`deploy/terraform/aws-light/README.md`](../deploy/terraform/aws-light/README.md)
— read it start to finish before running `terraform apply`, especially its
"What this is NOT" section.** This page only covers what carries over from
[`deployment.md`](deployment.md) unchanged, and the one thing that doesn't.

## What still applies, unchanged

The light stack ships the same three role images this repo builds, so every
one of [`deployment.md`](deployment.md)'s invariants about those images holds
here exactly as written — this stack does not get its own copies of them:

- The [two-role database model](deployment.md#the-two-role-database-model-required--read-this-first)
  and `db-bootstrap.sql` (run from the instance itself over SSM, since there's
  no separate bastion — see the light stack's own README step 5).
- [Configuration is env-only](deployment.md#configuration--everything-via-the-environment)
  and [first-boot bootstrap](deployment.md#first-boot-bootstrap-config) — the
  light stack's `margince.yaml` just lives in S3 instead of an EFS mount (its
  README step 3), fetched to local disk at boot.
- The [one-host routing table](deployment.md#routing) — nginx on the instance
  carries the same path rules the full stack's ALB listener rules do
  (`templates/nginx.conf.tftpl`), for the same reasons named there.
- [Health checks](deployment.md#health-checks) (`/healthz`, `/readyz`).
- The [same-release guard across all three roles](deployment.md#deploy-all-three-roles-at-one-release-the-guard-that-enforces-it)
  and [order of operations](deployment.md#order-of-operations) — deploying all
  three roles at once matters exactly as much on one instance as across three
  ECS services, since it's the images that carry the release stamp, not the
  compute shape running them.

## What's different: TLS

The full stack terminates TLS at the ALB with an ACM certificate. This stack
has no ALB, so its nginx obtains and renews its own Let's Encrypt certificate
via Certbot (`enable_tls`, default `true`) — which means the DNS-before-boot
step the full stack doesn't need: the instance's Elastic IP must already carry
an A record for `public_base_url`'s host before the instance's first boot, or
the ACME HTTP-01 challenge fails. The light stack's README walks through this
as an explicit numbered step, in order, before the instance is created.

Setting `enable_tls = false` keeps this stack on plain HTTP for an operator
fronting it with their own CDN, load balancer, or reverse proxy instead — the
same escape hatch the full stack's README documents for operators who don't
want its baseline WAF rules either.
