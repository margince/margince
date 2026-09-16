# Margince on AWS — light

One EC2 instance running api, worker, and web as docker compose services
behind a plain nginx reverse proxy, RDS for PostgreSQL, ElastiCache for
Redis, S3, Secrets Manager, ECR. No ALB, no ECS, no EFS, no customer-managed
KMS key, no NAT gateway. This is the minimal/small-deployment option — see
[the full stack](../aws/README.md) for the production-shaped alternative
this trades against.

## What this is NOT

Read this before you provision it:

- **TLS via Let's Encrypt on nginx itself, not a managed certificate.**
  `var.enable_tls` (default true) has this instance's own nginx obtain and
  renew its own certificate via Certbot — there's no ACM, no managed
  renewal, no CDN in front of it. That means the DNS-before-boot
  precondition in step 2 below, and it means an operator who wants a CDN,
  WAF, or their own TLS-terminating proxy in front of this instance instead
  sets `enable_tls = false` and fronts it themselves — this stack does not
  pick that for you, the same way it doesn't pick your domain.
- **No autoscaling, no multi-AZ compute.** One instance is all the compute
  this stack has. A traffic spike has nowhere to go; an instance failure
  means replacing the instance (`terraform taint aws_instance.this &&
  terraform apply`, or just `terraform apply` after an instance-level
  interruption AWS itself recovers from), not traffic shifting to a healthy
  peer, because there is no peer.
- **Single-AZ database and cache.** RDS runs `multi_az = false`; ElastiCache
  is one node, no automatic failover. Recovery from an underlying host
  failure is "restore from the last backup/snapshot," not seconds of
  failover.
- **No customer-managed KMS key.** Everything at rest (RDS, ElastiCache
  storage, S3, Secrets Manager) uses the relevant service's own AWS-managed
  default key instead of a CMK this stack owns — one fewer credential to
  grant and rotate, at the cost of the full stack's own CloudTrail
  key-usage trail and independent kill-switch.
- **No WAF, no VPC Flow Logs, no VPC endpoints, no Enhanced
  Monitoring/Performance Insights.** All real hardening/observability the
  full stack correctly pays for; this stack's reason to exist is not paying
  for them.

If any of the above is a hard requirement, use [`../aws/`](../aws/) instead.
This one is for a small deployment, a demo, or a cost-constrained
environment that accepts the tradeoffs above in plain terms.

## What this keeps, deliberately

Some floors aren't "light" enough to cut: encryption at rest everywhere
(under AWS-managed keys), `rds.force_ssl` + `sslmode=verify-full` on both
database DSNs (encrypted AND authenticated, not just encrypted), ElastiCache
transit encryption + AUTH token, the S3 bucket's `aws:SecureTransport`-deny
policy, `BucketOwnerEnforced` object ownership, IMDSv2-only on the instance,
no SSH ingress (shell access is via SSM Session Manager only), and IAM
scoped to exactly the ECR repos/secrets/S3 object this stack's own instance
needs — nothing broader.

## 1. Provision

```bash
cd deploy/terraform/aws-light
cp terraform.tfvars.example terraform.tfvars   # fill in public_base_url, admin_bootstrap_password, image_tag, letsencrypt_email
terraform init
terraform plan

# Everything EXCEPT the EC2 instance first. Two independent reasons to
# target past it: its own user-data pulls var.image_tag from the 3 ECR
# repos at boot (nothing has pushed that tag yet), and — when enable_tls is
# true (the default) — its first boot requests a Let's Encrypt certificate
# for public_base_url's host, which fails unless that DNS already points
# here (step 2). The Elastic IP is included in this targeted apply
# specifically so step 2 has an address to point DNS at before the instance
# (and its TLS bootstrap) exists at all.
terraform apply \
  -target=aws_ecr_repository.api -target=aws_ecr_repository.worker -target=aws_ecr_repository.web \
  -target=aws_db_instance.this -target=aws_elasticache_replication_group.this \
  -target=aws_s3_bucket.blobstore -target=aws_eip.this
```

This creates the VPC, RDS instance, ElastiCache node, S3 bucket, Secrets
Manager secrets, the 3 ECR repos, and the Elastic IP. Do steps 2–4 next,
then a final untargeted `terraform apply` creates the EC2 instance, which by
then has an image to pull and (if `enable_tls`) a DNS record its boot-time
certificate request can actually pass.

## 2. Point DNS at the Elastic IP

```bash
terraform output -raw instance_public_ip
```

Create an **A** record (not a CNAME — this is a static IP, not a hostname)
for `public_base_url`'s host pointing at that address, and wait for it to
resolve before continuing. Skip this only if you set `enable_tls = false`
and don't need this instance's own nginx to answer on the internet under
that name at all.

## 3. Upload `margince.yaml`

Unlike the full stack's EFS mount, this stack's config object lives in the
blobstore bucket itself, at a fixed key the instance's user-data fetches at
boot:

```bash
BUCKET="$(terraform output -raw s3_blobstore_bucket)"
aws s3 cp config/margince.example.yaml "s3://${BUCKET}/config/margince.yaml"
# edit locally first, or edit-then-recopy — set password_file to
# secrets/admin-password (the api's working dir is /app) per docs/deployment.md
```

To change it later: edit and re-`aws s3 cp` to the same key, then either
reboot the instance or `aws ssm start-session --target <instance-id>` and
run `sudo aws s3 cp "s3://$BUCKET/config/margince.yaml"
/opt/margince/config/margince.yaml && sudo systemctl restart margince`.

## 4. Push the three images

```bash
IMAGE_TAG="<the same value you set for image_tag in terraform.tfvars>"
PLATFORM="linux/arm64"   # match cpu_architecture — "linux/amd64" if you left it x86_64

aws ecr get-login-password --region "$(terraform output -raw aws_region)" \
  | docker login --username AWS --password-stdin "$(terraform output -raw ecr_api_repository_url | cut -d/ -f1)"

for role in api worker web; do
  docker buildx build --platform "$PLATFORM" --target "$role" \
    -t "$(terraform output -raw ecr_${role}_repository_url):${IMAGE_TAG}" --push .
done
```

The three ECR repos are `image_tag_mutability = IMMUTABLE`, same as the
full stack — pick a real release identifier, never `latest`.

## 5. Create the instance and bootstrap the database

```bash
terraform apply   # untargeted — creates the EC2 instance
```

The instance boots, pulls the three images, and starts the compose stack —
api/worker will crash-loop against RDS until the database roles exist,
which is expected and harmless (docker's `restart: unless-stopped` keeps
retrying). If `enable_tls` is true, the same first boot also requests the
Let's Encrypt certificate against the DNS record from step 2, as part of
the user-data script (`templates/user_data.sh.tpl`) — check it succeeded
with `aws ssm start-session --target <instance-id>` then
`sudo tail -100 /var/log/cloud-init-output.log`; a failure here almost
always means the DNS record isn't resolving yet or doesn't point at
`instance_public_ip`. Re-running the certificate request by hand after
fixing DNS: `sudo docker compose -f /opt/margince/docker-compose.yml run
--rm certbot certonly --webroot -w /var/www/certbot --email <your email>
-d <domain> --agree-tos --no-eff-email --non-interactive && sudo docker
compose -f /opt/margince/docker-compose.yml exec nginx nginx -s reload`.

Bootstrap the database once, from the instance itself (it already sits in the
same VPC as RDS, so there's no separate bastion step the way the full stack
needs):

```bash
aws ssm start-session --target "$(terraform output -raw instance_id)"

# Inside the session:
OWNER_PW="$(aws secretsmanager get-secret-value --secret-id "$(terraform output -json secret_arns | jq -r .owner_dsn)" --query SecretString --output text | sed -E 's#.*:([^:@]+)@.*#\1#')"
APP_PW="$(aws secretsmanager get-secret-value --secret-id "$(terraform output -json secret_arns | jq -r .app_dsn)" --query SecretString --output text | sed -E 's#.*:([^:@]+)@.*#\1#')"

sudo psql "postgres://dbadmin:<terraform state show random_password.rds_master's result>@$(terraform output -raw rds_endpoint):5432/margince?sslmode=verify-full&sslrootcert=/opt/margince/config/rds-ca-bundle.pem" \
  -v owner_pw="$OWNER_PW" -v app_pw="$APP_PW" \
  -f /opt/margince/db-bootstrap.sql
```

`scripts/deploy/db-bootstrap.sql` isn't on the instance by default — copy it
up first (`aws s3 cp` via the blobstore bucket, or paste it in over the SSM
session) since there's no git checkout on this box. Once bootstrapped, the
already-running containers reconnect on their own next retry — no restart
needed, though `sudo systemctl restart margince` forces it immediately.

## 6. First login

DNS is already pointed at the instance (step 2), so once nginx can reach a
healthy `/healthz` on `api`, the api applies migrations and bootstraps the
organization from `MARGINCE_ADMIN_PASSWORD` — after which, per
`docs/deployment.md`, remove `bootstrap_admin` from `margince.yaml` and
rotate the `admin_password` secret to something inert.

## 7. Releasing a new version

Build/push new images tagged with the release version (step 4), SSM into
the instance, and:

```bash
sudo aws ecr get-login-password --region <region> | sudo docker login --username AWS --password-stdin <registry>
sudo docker compose -f /opt/margince/docker-compose.yml pull
sudo systemctl restart margince
```

There is no rolling deploy here — this briefly stops all three containers
during the restart, unlike the full stack's ECS circuit-breaker rollout.
That's the tradeoff of one instance with no peer to shift traffic to.

## Security posture

See ["What this keeps, deliberately"](#what-this-keeps-deliberately) above
for the floors, and ["What this is NOT"](#what-this-is-not) for the
tradeoffs. A few specifics worth calling out:

- **IMDSv2 only** (`ec2.tf`'s `metadata_options`) — the instance role can
  reach Secrets Manager, ECR, and S3; IMDSv1's token-less metadata GET is
  exactly what an SSRF-shaped bug on this box would try first.
- **No SSH ingress.** The security group (`network.tf`) has no port 22
  rule; the instance role carries `AmazonSSMManagedInstanceCore` instead
  (`iam.tf`), so shell access is via `aws ssm start-session`, logged and
  IAM-gated rather than a key anyone with the private key can use.
- **Secrets never touch Terraform state in plaintext on the instance.**
  They're fetched at boot via the instance role's scoped
  `secretsmanager:GetSecretValue` grant straight into `/opt/margince/.env`
  (mode 600) — never written to a log, an S3 object, or the compose file
  itself.
- **The blobstore IAM user** (`s3.tf`) is a static-key user, not the
  instance role, for the same reason the full stack uses one: the
  minio-go client authenticates with `credentials.NewStaticV4`, which
  never reads the instance's own role credentials.
- **TLS certificate renewal is self-managed, not AWS-managed.** No ACM here
  — `margince-renew.timer` runs `certbot renew` twice daily (Certbot's own
  recommended cadence) and reloads nginx on success. If the instance is
  ever replaced (`terraform taint aws_instance.this`), the certificate is
  re-issued from scratch on the new instance's first boot rather than
  carried over — same tradeoff as every other piece of this instance's
  local state.

## Deliberately not done (beyond "What this is NOT")

- **Secrets Manager rotation** for the DSNs / blobstore access key — same
  gap the full stack names in its own README, for the same reason (these
  secrets aren't shaped for AWS's canned rotation Lambdas).
- **Remote state.** `versions.tf`'s commented `backend "s3"` block is yours
  to fill in before calling any use of this stack beyond a one-off
  `terraform plan` — every `random_password` result in this state file is
  currently sitting in plaintext wherever you run `terraform apply`.
- **AWS Backup / EFS.** No EFS in this stack at all — the one config object
  it needs lives in the blobstore bucket instead, versioned there.
