# Terraform: cloud deployment stacks

Deployment-target Terraform root modules — one per cloud, starting with
**AWS** — that stand up a complete Margince installation: the three role
images (`api`, `worker`, `web`), a managed Postgres with `pgvector`, managed
Redis, S3-compatible object storage, a secrets store with customer-managed
encryption, and one routed host in front of `api` + `web` per the table in
[`docs/deployment.md`](../../docs/deployment.md#routing).

GCP and Azure are not built yet — this is deliberately AWS-first, not
AWS-only forever. A later PR adds `deploy/terraform/gcp/` and
`deploy/terraform/azure/` following the same shape.

## Naming the conflict

[`docs/deployment.md`](../../docs/deployment.md) states the product's shipped
position: "This repo carries only the **generic** pieces; a concrete
deployment (its domain, secrets, platform manifests) is yours to own — keep
those in your own infra repo." This stack is exactly that concrete,
platform-specific manifest set, requested directly rather than kept in a
separate infra repo.

What that costs: this stack will drift the way any infra-as-code drifts —
provider resource renames, a new pgvector-capable engine version, a changed
default in the underlying Terraform provider — and nothing here re-derives
that contract from the product the way `backend/gates/*` re-derive theirs from
the Go tree. There is no gate that fails when `MARGINCE_DSN` or the routing
table changes shape and this `.tf` tree does not follow. Treat it as a
**reference starting point** for an operator's own fork, not a promise of
ongoing parity — whether a team wants that parity enforced is
`status: needs-decision` territory; absent that decision this is "fix it,
don't file it" scoped to what shipped today.

## Shape

Serverless containers (ECS Fargate — no cluster to operate) and a full
managed stack (managed Postgres, managed Redis, object storage, secrets
manager, image registry, one customer-managed KMS key over all of it — all
provisioned by Terraform). See [`aws/README.md`](aws/README.md) for the full
resource list, the security posture, and how to use it.

## What this does NOT cover

Autoscaling policies beyond a fixed desired count, multi-region/HA, a WAF in
front of the routing layer, and disaster recovery / backup-restore runbooks
are out of scope — each is a real decision (retention window, RPO/RTO, which
regions) that belongs to the operator standing this up, not a default this
reference should pick for them.

## Using the stack

```bash
cd deploy/terraform/aws
cp terraform.tfvars.example terraform.tfvars
# edit terraform.tfvars
terraform init
terraform plan
terraform apply
```

**Local state is the default, and it is not the recommended posture beyond a
one-off `terraform plan`.** Every stack's `versions.tf` carries a commented
`backend "s3"` block — uncomment and fill it in with your own state bucket
before an `apply` you intend to keep, or `terraform apply` writes RDS,
Redis, and every generated credential from `secrets.tf` into a plaintext
file on whatever machine ran it.

Then follow [`aws/README.md`](aws/README.md) for the one-time steps Terraform
does not do: bootstrapping the database roles
(`scripts/deploy/db-bootstrap.sql`), mounting `margince.yaml`, and
building/pushing the three images to the registry Terraform created.
