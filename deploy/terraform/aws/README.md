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
terraform apply
```

This creates the VPC, KMS key, RDS instance, ElastiCache replication group,
S3 bucket, EFS filesystem, Secrets Manager secrets, the 3 ECR repos, and an
ECS cluster — but the ECS **services** will not reach a healthy state yet:
there is no image at `image_tag` in the repos, and the database has no
`margince_owner`/`margince_app` roles yet. Do steps 2–4 next, then re-apply
if you changed `image_tag`.

## 2. Bootstrap the database (once)

RDS's master user is `dbadmin` (see `rds.tf` for why it is not named
`margince_owner`). From a host with network access to the RDS instance's
private endpoint (a bastion, a Cloud9/SSM-connected instance, or a one-off ECS
task in the same VPC — the instance has no public IP):

```bash
OWNER_PW="$(aws secretsmanager get-secret-value --secret-id "$(terraform output -json secret_arns | jq -r .owner_dsn)" --query SecretString --output text | sed -E 's#.*:([^:@]+)@.*#\1#')"
APP_PW="$(aws secretsmanager get-secret-value --secret-id "$(terraform output -json secret_arns | jq -r .app_dsn)" --query SecretString --output text | sed -E 's#.*:([^:@]+)@.*#\1#')"
MASTER_PW="$(terraform state show random_password.rds_master | grep 'result ' | awk '{print $3}' | tr -d '"')"

psql "postgres://dbadmin:${MASTER_PW}@$(terraform output -raw rds_endpoint):5432/margince?sslmode=require" \
  -v owner_pw="$OWNER_PW" -v app_pw="$APP_PW" \
  -f ../../../scripts/deploy/db-bootstrap.sql
```

(`aws rds describe-db-instances` never returns the master password; it only
exists as this Terraform-generated value.)

## 3. Push the three images

```bash
IMAGE_TAG="<the same value you set for image_tag in terraform.tfvars>"

aws ecr get-login-password --region "$(terraform output -raw ecr_api_repository_url | cut -d. -f4)" \
  | docker login --username AWS --password-stdin "$(terraform output -raw ecr_api_repository_url | cut -d/ -f1)"

for role in api worker web; do
  docker build --target "$role" -t "$(terraform output -raw ecr_${role}_repository_url):${IMAGE_TAG}" .
  docker push "$(terraform output -raw ecr_${role}_repository_url):${IMAGE_TAG}"
done
```

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
| Client → ALB | TLS 1.3 only (`ELBSecurityPolicy-TLS13-1-2-2021-06`), HTTP redirects to HTTPS |
| ALB → api/web tasks | Plaintext HTTP inside the VPC's private subnets — matches the product's own architecture: `cmd/api` serves plain HTTP and terminates TLS ahead of itself (`docs/reference/configuration.md`) |
| Task → RDS | `rds.force_ssl=1` (server refuses plaintext) + `sslmode=require` on both DSNs (client never attempts it) |
| Task → ElastiCache | `transit_encryption_enabled = true` + auth token |
| Task → EFS | `transit_encryption = "ENABLED"` on the mount |
| Task → S3 | Bucket policy denies any request where `aws:SecureTransport = false`, independent of the client's own `MARGINCE_BLOBSTORE_USE_SSL` setting |

**Other hardening in this stack**: ECR repos are `image_tag_mutability =
IMMUTABLE` (a pushed tag can't be silently overwritten) with a lifecycle
policy expiring untagged images after 14 days; the `db`/`redis`/`efs`
security groups carry no egress rule at all (they never originate outbound
traffic, so allow-all egress bought nothing); the `web` ECS task uses its
own execution role with no Secrets Manager access, since it reads no
secrets — only `api` and `worker`'s shared execution role can; the S3
bucket has `object_ownership = BucketOwnerEnforced` (ACLs disabled outright,
so access runs through IAM/bucket policy alone); the api and worker task
definitions set `stopTimeout = 60` so an in-flight request or job finishes
draining rather than being cut off at Fargate's 30s default; every task
definition declares `runtime_platform` explicitly (`var.cpu_architecture`,
default `X86_64` — `ARM64` is genuinely supported, not theoretical, see the
variable's own description).

**Left out, deliberately** (see the [shared README](../README.md)): S3
object versioning / MFA delete, VPC endpoints for ECR/S3/Secrets Manager
(all egress currently transits the NAT gateway), a WAF in front of the ALB.
Each is a real option, not a gap this stack missed — they cost something
(storage, a VPC endpoint's own security-group surface, WAF rule tuning)
that belongs to a deployment decision rather than a default.
