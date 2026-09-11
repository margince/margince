variable "aws_region" {
  description = "AWS region every resource is created in."
  type        = string
  default     = "eu-central-1"
}

variable "name_prefix" {
  description = "Short prefix for every resource name (e.g. \"margince-prod\")."
  type        = string
  default     = "margince"
}

variable "environment" {
  description = <<-EOT
    Stamped onto every resource's Environment tag (provider default_tags,
    versions.tf) — the dimension a cost/operations tool groups this stack's
    spend and automation by when the same name_prefix is reused across more
    than one environment (a staging copy of "margince", say).
  EOT
  type        = string
  default     = "production"
}

variable "vpc_cidr" {
  description = "CIDR block for the VPC this stack creates."
  type        = string
  default     = "10.20.0.0/16"
}

variable "az_count" {
  description = "Number of availability zones to spread public/private subnets across."
  type        = number
  default     = 2
  validation {
    # RDS and ElastiCache subnet groups both require subnets in at least two
    # AZs — az_count = 1 produces a db_subnet_group covering one, which RDS
    # refuses at CreateDBSubnetGroup, not at plan time, so this catches it
    # before the apply gets that far.
    condition     = var.az_count >= 2
    error_message = "az_count must be at least 2 — RDS and ElastiCache subnet groups both require two Availability Zones."
  }
}

variable "cpu_architecture" {
  description = <<-EOT
    Fargate runtime_platform.cpu_architecture for all three task definitions
    — "X86_64" or "ARM64". Defaults to ARM64 (Graviton): RDS (db.t4g.medium)
    and ElastiCache (cache.t4g.small) already default to Graviton instance
    families, the product's own release pipeline already builds and
    smoke-tests every image on real arm64 GitHub runners
    (.github/workflows/release.yml) and pushes multi-arch
    (linux/amd64,linux/arm64) images, the backend has zero cgo, and the
    Dockerfile cross-compiles via TARGETARCH already — Fargate on Graviton
    also runs meaningfully cheaper than x86_64 at the same vCPU/memory. Set
    to X86_64 if your own build/push step (README.md step 3) only produces
    an amd64 image.
  EOT
  type        = string
  default     = "ARM64"
  validation {
    condition     = contains(["X86_64", "ARM64"], var.cpu_architecture)
    error_message = "cpu_architecture must be \"X86_64\" or \"ARM64\"."
  }
}

# ---- Images -------------------------------------------------------------

variable "image_tag" {
  description = <<-EOT
    Tag to deploy for all three roles (api, worker, web) — a real release
    version (e.g. a git SHA or MARGINCE_RELEASE_VERSION), never "latest".
    The ECR repos are image_tag_mutability = IMMUTABLE (ecs.tf), so a tag can
    only ever be pushed once; "latest" would work for exactly one release and
    then refuse every push after it, which is the immutability doing its job
    rather than a bug. Push the tag to the ECR repos this stack creates
    before the first `terraform apply` that references it — ECS refuses to
    start a task against a tag that does not exist yet. No default: picking a
    floating tag is an operator decision this stack should not make silently.
  EOT
  type        = string
  validation {
    # An empty or whitespace image_tag builds "repo:" — no tag at all — which
    # ECS rejects rather than defaulting to anything, so this fails fast at
    # plan time with a message that names the actual problem. "latest" is
    # deliberately still admitted: it is valid for exactly one push to an
    # IMMUTABLE repo (see the description above), which is a release-policy
    # violation this stack warns about elsewhere, not a value that breaks
    # the deployment outright.
    condition     = length(trimspace(var.image_tag)) > 0
    error_message = "image_tag must not be empty or whitespace — ECS needs repository:tag, not repository:."
  }
}

# ---- Compute sizing -------------------------------------------------------

variable "api_desired_count" {
  type    = number
  default = 2
}

variable "worker_desired_count" {
  type    = number
  default = 1
}

variable "api_autoscaling_max_count" {
  description = "Ceiling for api's CPU-based Application Auto Scaling target (ecs.tf) — api_desired_count is the floor."
  type        = number
  default     = 4
}

variable "worker_autoscaling_max_count" {
  description = "Ceiling for worker's CPU-based Application Auto Scaling target (ecs.tf) — worker_desired_count is the floor."
  type        = number
  default     = 3
}

variable "web_desired_count" {
  type    = number
  default = 2
}

variable "api_cpu" {
  type    = number
  default = 512
}

variable "api_memory" {
  type    = number
  default = 1024
}

variable "worker_cpu" {
  type    = number
  default = 512
}

variable "worker_memory" {
  type    = number
  default = 1024
}

variable "web_cpu" {
  type    = number
  default = 256
}

variable "web_memory" {
  type    = number
  default = 512
}

variable "log_retention_days" {
  type    = number
  default = 30
}

variable "ecr_tagged_image_retain_count" {
  description = <<-EOT
    ecs.tf's ECR lifecycle policy expires all but the most recent N tagged
    (released) images per repo, once IMMUTABLE tag mutability (ecs.tf) means
    none of them are ever reclaimed by a later push to the same tag. 30 is a
    generous rollback window for a CRM's release cadence, not a tuned value
    — raise it if you release more often than that and still want that many
    rollback targets on hand.
  EOT
  type        = number
  default     = 30
}

# ---- Database ---------------------------------------------------------------

variable "db_instance_class" {
  type    = string
  default = "db.t4g.medium"
}

variable "db_allocated_storage_gb" {
  type    = number
  default = 50
}

variable "db_engine_version" {
  description = "Postgres major/minor version. Must be a version RDS lists pgvector support for."
  type        = string
  default     = "16.4"
}

variable "db_multi_az" {
  type    = bool
  default = true
}

variable "db_backup_retention_days" {
  type    = number
  default = 7
}

# ---- Redis ------------------------------------------------------------------

variable "redis_node_type" {
  type    = string
  default = "cache.t4g.small"
}

# ---- Routing ------------------------------------------------------------------

variable "acm_certificate_arn" {
  description = <<-EOT
    ARN of an ACM certificate covering the public host this installation
    serves (MARGINCE_PUBLIC_BASE_URL's host). Not created by this stack —
    validating a certificate needs the domain's own DNS, which lives
    wherever the operator's zone lives.
  EOT
  type        = string
}

variable "public_base_url" {
  description = "MARGINCE_PUBLIC_BASE_URL — e.g. https://crm.example.com"
  type        = string
}

# ---- Secrets and application config -----------------------------------------

variable "license_token" {
  description = "MARGINCE_LICENSE. Empty runs unlicensed, which a production role refuses to boot on."
  type        = string
  default     = ""
  sensitive   = true
}

variable "admin_bootstrap_password" {
  description = <<-EOT
    MARGINCE_ADMIN_PASSWORD for the first boot against an empty database.
    Rotate/remove per docs/deployment.md once the organization exists —
    this variable only seeds the initial secret version.
  EOT
  type        = string
  sensitive   = true
}
