# Margince on AWS

ECS Fargate (api, worker, web), RDS for PostgreSQL, ElastiCache for Redis, S3,
EFS (for the mounted `margince.yaml`), Secrets Manager, one customer-managed
KMS key, one ALB. See the [shared README](../README.md) for the cross-cloud
design notes and what is deliberately out of scope (autoscaling policies,
multi-region/HA, WAF, DR runbooks).

## 1. Provision

```bash
cd deploy/terraform/aws
cp terraform.tfvars.example terraform.tfvars   # fill in acm_certificate_arn, public_base_url, admin_bootstrap_password, image_tag
terraform init
terraform plan

# Everything EXCEPT the 3 ECS services first — they reference image_tag,
# and nothing has pushed it yet. A plain `terraform apply` here creates the
# services anyway, pointed at a tag ECR does not have, and they sit
# unhealthy until you catch up with steps 2-4 below and re-apply. Targeting
# past them avoids that round trip entirely; it is not required, just
# cheaper than watching ECS retry a pull that cannot succeed yet.
terraform apply \
  -target=aws_ecr_repository.api -target=aws_ecr_repository.worker -target=aws_ecr_repository.web \
  -target=aws_db_instance.this -target=aws_elasticache_replication_group.this \
  -target=aws_s3_bucket.blobstore -target=aws_efs_file_system.config
```

This creates the VPC, KMS key, RDS instance, ElastiCache replication group,
S3 bucket, EFS filesystem, Secrets Manager secrets, and the 3 ECR repos. Do
steps 2–4 next — bootstrap the database, push the images, mount
`margince.yaml` — then run a final untargeted `terraform apply` to create
the ALB and the 3 ECS services, which by then have an image to pull and a
database to migrate against.

## 2. Bootstrap the database (once)

RDS's master user is `dbadmin` (see `rds.tf` for why it is not named
`margince_owner`). From a host with network access to the RDS instance's
private endpoint (a bastion, a Cloud9/SSM-connected instance, or a one-off ECS
task in the same VPC — the instance has no public IP):

```bash
curl -o /tmp/rds-ca-bundle.pem https://truststore.pki.rds.amazonaws.com/global/global-bundle.pem

OWNER_PW="$(aws secretsmanager get-secret-value --secret-id "$(terraform output -json secret_arns | jq -r .owner_dsn)" --query SecretString --output text | sed -E 's#.*:([^:@]+)@.*#\1#')"
APP_PW="$(aws secretsmanager get-secret-value --secret-id "$(terraform output -json secret_arns | jq -r .app_dsn)" --query SecretString --output text | sed -E 's#.*:([^:@]+)@.*#\1#')"
MASTER_PW="$(terraform state show random_password.rds_master | grep 'result ' | awk '{print $3}' | tr -d '"')"

psql "postgres://dbadmin:${MASTER_PW}@$(terraform output -raw rds_endpoint):5432/margince?sslmode=verify-full&sslrootcert=/tmp/rds-ca-bundle.pem" \
  -v owner_pw="$OWNER_PW" -v app_pw="$APP_PW" \
  -f ../../../scripts/deploy/db-bootstrap.sql
```

(`aws rds describe-db-instances` never returns the master password; it only
exists as this Terraform-generated value.)

## 3. Push the three images

```bash
IMAGE_TAG="<the same value you set for image_tag in terraform.tfvars>"
PLATFORM="linux/arm64"   # match cpu_architecture in terraform.tfvars — "linux/amd64" if you left it X86_64

aws ecr get-login-password --region "$(terraform output -raw ecr_api_repository_url | cut -d. -f4)" \
  | docker login --username AWS --password-stdin "$(terraform output -raw ecr_api_repository_url | cut -d/ -f1)"

for role in api worker web; do
  docker buildx build --platform "$PLATFORM" --target "$role" \
    -t "$(terraform output -raw ecr_${role}_repository_url):${IMAGE_TAG}" --push .
done
```

A plain `docker build` produces an image matching your OWN machine's
architecture, not necessarily the one `cpu_architecture` names — `buildx
--platform` is what actually cross-compiles to it (the Dockerfile already
supports this via `TARGETARCH`; nothing here needs to change).

`IMAGE_TAG` must equal `var.image_tag` exactly. The three ECR repos are
`image_tag_mutability = IMMUTABLE`, so pick a real release identifier (a git
SHA, `MARGINCE_RELEASE_VERSION`) rather than `latest` — a tag can be pushed
exactly once; re-pushing it (the usual `latest` workflow) is refused by
design, not a bug.

## 4. Mount `margince.yaml` onto EFS (once)

Terraform provisions the EFS filesystem and access point; it does not write
into it. From any instance in the VPC with the `amazon-efs-utils` package (a
temporary EC2 instance in a public subnet is the simplest path):

```bash
sudo mount -t efs -o tls,accesspoint="$(terraform output -raw efs_config_access_point_id)" \
  "$(terraform output -raw efs_file_system_id)":/ /mnt/margince-config
sudo cp config/margince.example.yaml /mnt/margince-config/margince.yaml
# edit /mnt/margince-config/margince.yaml — set password_file to
# secrets/admin-password (the api's working dir is /app) per docs/deployment.md

# The api and worker DSNs (secrets.tf) name this file at
# /app/config/rds-ca-bundle.pem — the same mount, so it goes on beside
# margince.yaml rather than needing a mount of its own.
sudo cp /tmp/rds-ca-bundle.pem /mnt/margince-config/rds-ca-bundle.pem

sudo umount /mnt/margince-config
```

## 5. DNS + first boot

Point `public_base_url`'s host at `terraform output -raw alb_dns_name` (a CNAME
or an ALIAS record) and confirm `acm_certificate_arn` covers that host. Once
the api task can reach a healthy `/healthz` on the target group, it applies
migrations and bootstraps the organization from `MARGINCE_ADMIN_PASSWORD` —
after which, per `docs/deployment.md`, remove `bootstrap_admin` from
`margince.yaml` and rotate the `admin_password` secret to something inert.

## 6. Releasing a new version

Build/push new images tagged with the release version, set `image_tag` to
that version, `terraform apply`. All three ECS services pick up the new task
definition on the same apply — `docs/deployment.md`'s release-version guard
means api/worker/web should always move together; applying only one role's
change (e.g. hand-editing a service's desired count without touching
`image_tag`) does not trigger a new deployment for the others.

### Turning on the Redis-TLS / S3-SSE-KMS enforcement, safely

`elasticache.tf`'s `transit_encryption_mode = "required"` and `s3.tf`'s
`DenyWrongEncryption`/`DenyWrongKMSKey` bucket-policy statements only work
because the api/worker images now negotiate TLS and send an SSE-KMS header
(`MARGINCE_REDIS_TLS`, `MARGINCE_BLOBSTORE_KMS_KEY_ID`, both in `ecs.tf`).
Terraform has no way to express "wait until every old task has drained" —
`aws_ecs_service` returns as soon as the API call to update it succeeds, not
once the rollout finishes — so a single untargeted `apply` can flip
ElastiCache to `required` or the S3 policy to enforcing while an OLD task
revision (no TLS, no SSE header) is still serving traffic. That old task
loses Redis connectivity, or has every upload denied, until it's replaced.

Two-step apply avoids it:

```bash
# 1. Roll the new images out and WAIT for the rollout to finish before
#    touching ElastiCache/S3 enforcement.
CLUSTER="$(terraform output -raw ecs_cluster_name)"
PREFIX="${CLUSTER%-cluster}"   # cluster is "${name_prefix}-cluster"; services are "${name_prefix}-api"/"-worker"
terraform apply -target=aws_ecs_service.api -target=aws_ecs_service.worker
aws ecs wait services-stable --cluster "$CLUSTER" --services "${PREFIX}-api" "${PREFIX}-worker"

# 2. Only now apply everything else — this is what actually flips
#    transit_encryption_mode and the S3 deny statements live.
terraform apply
```

This only matters the FIRST time you turn either flag on (or after any gap
where an old, non-TLS/non-SSE image was running). A steady-state release
that already has both flags set can apply untargeted as usual.

## Security posture

**Encryption at rest** — one customer-managed KMS key (`kms.tf`, rotation
enabled) covers everything this stack stores: RDS, ElastiCache, S3 (SSE-KMS
with Bucket Keys), EFS, every Secrets Manager secret, and all 3 ECR repos.
IAM grants are scoped to exactly who needs the key — the ECS execution role
(Secrets Manager reads + its own ECR image), `execution_web`'s own narrower
grant (its ECR image only, no secrets), and the blobstore IAM user (S3
object encrypt/decrypt) — nobody else can use it. One key, not one per
service: see `kms.tf` for why a single CMK is the right blast-radius
boundary here rather than six to separately grant.

**Encryption in transit** — every hop is enforced, not just requested:

| Hop | Enforcement |
|---|---|
| Client → ALB | TLS 1.2 and TLS 1.3 (`ELBSecurityPolicy-TLS13-1-2-2021-06` — the name is the policy's, not a claim that 1.2 is refused), HTTP redirects to HTTPS |
| ALB → api/web tasks | Plaintext HTTP inside the VPC's private subnets — matches the product's own architecture: `cmd/api` serves plain HTTP and terminates TLS ahead of itself (`docs/reference/configuration.md`) |
| Task → RDS | `rds.force_ssl=1` (server refuses plaintext) + `sslmode=verify-full` on both DSNs — encrypted AND authenticated against the RDS CA bundle (step 4), not merely encrypted; `sslmode=require` alone lets pgx accept any certificate, including an attacker's |
| Task → ElastiCache | `transit_encryption_enabled = true`, `transit_encryption_mode = "preferred"` (not `"required"`) + auth token. `"preferred"` rather than the stricter default because the product's Redis client (`backend/internal/platform/events/relay.go`) sets no `TLSConfig` at all — `"required"` would refuse every connection this app actually makes. The real fix is in the Go client; this is the honest floor until it lands, not a claim the wire is protected end to end |
| Task → EFS | `transit_encryption = "ENABLED"` on the mount |
| Task → S3 | Bucket policy denies any request where `aws:SecureTransport = false`, independent of the client's own `MARGINCE_BLOBSTORE_USE_SSL` setting |

**Other hardening in this stack**: ECR repos are `image_tag_mutability =
IMMUTABLE` (a pushed tag can't be silently overwritten) with a lifecycle
policy expiring untagged images after 14 days; the `db`/`redis`/`efs`
security groups carry no egress rule at all (they never originate outbound
traffic, so allow-all egress bought nothing); `ecs_tasks`' own egress is
scoped to in-VPC traffic plus the specific external ports the product
genuinely calls out on (443 HTTPS, 25/465/587 SMTP) rather than every
port/protocol to anywhere; VPC endpoints (S3 Gateway + Interface endpoints
for ECR/Secrets Manager/KMS/CloudWatch Logs, `vpc-endpoints.tf`) keep that
AWS-internal traffic off the NAT/public path entirely; the `web` ECS task
uses its own execution role with no Secrets Manager access, since it reads
no secrets — only `api` and `worker`'s shared execution role can, and
neither execution role carries the `AmazonECSTaskExecutionRolePolicy`
managed policy (its `Resource: "*"` ECR/logs grants would have overridden
the scoped statements sitting next to it, not narrowed them); the S3
bucket has `object_ownership = BucketOwnerEnforced` (ACLs disabled outright,
so access runs through IAM/bucket policy alone); the api and worker task
definitions set `stopTimeout = 60` so an in-flight request or job finishes
draining rather than being cut off at Fargate's 30s default; every task
definition declares `runtime_platform` explicitly (`var.cpu_architecture`,
default `ARM64` — RDS and ElastiCache already default to Graviton instance
families, so this keeps the whole stack on one architecture family by
default; see the variable's own description).

**S3 SSE-KMS enforcement**: `s3.tf`'s bucket policy denies any `PutObject`
that isn't `aws:kms`-encrypted under this stack's own key
(`MARGINCE_BLOBSTORE_KMS_KEY_ID`, wired in `ecs.tf`) — paired with the Go
change in `backend/internal/platform/blobstore/s3.go` that sends the
matching SSE-KMS header on every write. Both sides shipped together;
landing the policy alone would have refused every upload the app makes.

**S3 versioning** is enabled with a 90-day noncurrent-version expiry, so an
accidental delete/overwrite on this CRM's attachment store is recoverable.

**Left out, deliberately** (see the [shared README](../README.md)): S3
Object Lock / MFA delete, a WAF in front of the ALB. Each is a real option,
not a gap this stack missed — they cost something (a stricter retention
posture, WAF rule tuning) that belongs to a deployment decision rather than
a default.
