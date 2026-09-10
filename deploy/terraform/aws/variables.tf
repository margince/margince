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

variable "vpc_cidr" {
  description = "CIDR block for the VPC this stack creates."
  type        = string
  default     = "10.20.0.0/16"
}

variable "az_count" {
  description = "Number of availability zones to spread public/private subnets across."
  type        = number
  default     = 2
}

variable "cpu_architecture" {
  description = <<-EOT
    Fargate runtime_platform.cpu_architecture for all three task definitions
    — "X86_64" or "ARM64". ARM64 (Graviton) is genuinely supported here, not
    theoretical: the product's own release pipeline already builds and
    smoke-tests every image on real arm64 GitHub runners
    (.github/workflows/release.yml), the backend has zero cgo, and the
    Dockerfile cross-compiles via TARGETARCH already. Defaults to X86_64
    because it is the safer unsurprising default for a reference stack, not
    because arm64 is unproven — switch it once you've pushed arm64 (or
    multi-arch) images to the ECR repos this stack creates.
  EOT
  type        = string
  default     = "X86_64"
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
