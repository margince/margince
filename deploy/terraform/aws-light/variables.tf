variable "aws_region" {
  description = "AWS region every resource is created in."
  type        = string
  default     = "eu-central-1"
}

variable "name_prefix" {
  description = "Short prefix for every resource name (e.g. \"margince-light\")."
  type        = string
  default     = "margince-light"
}

variable "environment" {
  description = <<-EOT
    Stamped onto every resource's Environment tag (provider default_tags,
    versions.tf) — the dimension a cost/operations tool groups this stack's
    spend by when the same name_prefix is reused across more than one
    environment.
  EOT
  type        = string
  default     = "production"
}

variable "vpc_cidr" {
  description = "CIDR block for the VPC this stack creates."
  type        = string
  default     = "10.30.0.0/16"
}

variable "az_count" {
  description = <<-EOT
    Number of AZs the two PRIVATE subnets (RDS/ElastiCache subnet groups)
    spread across. The compute itself is one EC2 instance in one public
    subnet, always — this only sizes the private side. RDS and ElastiCache
    subnet groups both require subnets in at least two AZs even for a
    single-AZ instance/single-node cache, which is why this floor is 2
    rather than 1 even though nothing here runs Multi-AZ.
  EOT
  type        = number
  default     = 2
  validation {
    condition     = var.az_count >= 2
    error_message = "az_count must be at least 2 — RDS and ElastiCache subnet groups both require two Availability Zones."
  }
}

variable "cpu_architecture" {
  description = <<-EOT
    Instance/image architecture — "x86_64" or "arm64". Defaults to arm64
    (Graviton, e.g. t4g.small): RDS (db.t4g.micro) and ElastiCache
    (cache.t4g.micro) already default to Graviton families, the product's
    own release pipeline builds and smoke-tests every image on real arm64
    GitHub runners and pushes multi-arch (linux/amd64,linux/arm64) images,
    and the backend has zero cgo. Set to "x86_64" if your own build/push
    step only produces an amd64 image — this also picks the matching
    Amazon Linux 2023 AMI via the SSM parameter in ec2.tf.
  EOT
  type        = string
  default     = "arm64"
  validation {
    condition     = contains(["x86_64", "arm64"], var.cpu_architecture)
    error_message = "cpu_architecture must be \"x86_64\" or \"arm64\"."
  }
}

# ---- Images -------------------------------------------------------------

variable "image_tag" {
  description = <<-EOT
    Tag to deploy for all three roles (api, worker, web) — a real release
    version (e.g. a git SHA or MARGINCE_RELEASE_VERSION), never "latest".
    The ECR repos are image_tag_mutability = IMMUTABLE (ecr.tf), so a tag can
    only ever be pushed once. Push the tag to the ECR repos this stack
    creates before the first boot that references it — the instance's own
    user-data (ec2.tf) refuses to start the compose stack against a tag it
    cannot pull. No default: picking a floating tag is an operator decision
    this stack should not make silently.
  EOT
  type        = string
  validation {
    condition     = length(trimspace(var.image_tag)) > 0
    error_message = "image_tag must not be empty or whitespace — the compose stack needs repository:tag, not repository:."
  }
}

# ---- Compute sizing -------------------------------------------------------

variable "instance_type" {
  description = <<-EOT
    Single EC2 instance running api + worker + web + the reverse-proxy
    container (ec2.tf) — all of this stack's compute, nowhere else to
    spread load. t4g.small (2 vCPU / 2GiB) is the floor that fits all four
    containers with headroom for the OS and docker itself; t4g.micro (1GiB)
    was tried and starved the api's own in-process caches under any real
    traffic. Burstable (T-family): fine for a light/small-deployment
    workload, not for one under sustained load — size up (m7g family) if
    CPU credit balance becomes the bottleneck. Must match cpu_architecture:
    Graviton families end in "g" before the size suffix (t4g, m7g, etc.).
  EOT
  type        = string
  default     = "t4g.small"

  validation {
    condition     = var.cpu_architecture != "x86_64" || !can(regex("^[a-z][0-9]g\\.", var.instance_type))
    error_message = "instance_type \"${var.instance_type}\" is a Graviton (arm64) family; set cpu_architecture = \"arm64\" or pick an x86_64 instance type."
  }
  validation {
    condition     = var.cpu_architecture != "arm64" || can(regex("^[a-z][0-9]g\\.", var.instance_type))
    error_message = "instance_type \"${var.instance_type}\" does not look like a Graviton (arm64) family (e.g. t4g.small, m7g.large); set cpu_architecture = \"x86_64\" or choose an arm64 instance type."
  }
}

variable "root_volume_gb" {
  type    = number
  default = 30
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

variable "license_token" {
  description = "MARGINCE_LICENSE. Empty runs unlicensed, which a production role refuses to boot on."
  type        = string
  default     = ""
  sensitive   = true
}

variable "public_base_url" {
  description = <<-EOT
    MARGINCE_PUBLIC_BASE_URL, e.g. https://crm.example.com. Its host is what
    nginx serves TLS for and what certbot requests a certificate for
    (ec2.tf's `local.domain`) when var.enable_tls is true, so this domain's
    DNS must already resolve to instance_public_ip (an A record) before
    first boot — see README step 2. Use http:// only if enable_tls is
    false and you're fronting this instance with your own TLS-terminating
    reverse proxy or CDN instead.
  EOT
  type        = string
}

variable "enable_tls" {
  description = <<-EOT
    On by default: nginx gets a Let's Encrypt certificate for
    public_base_url's host via certbot's webroot plugin
    (templates/user_data.sh.tpl) and serves HTTPS on :443, redirecting :80.
    Requires that host's DNS to already point at instance_public_ip before
    the instance's first boot, or the ACME HTTP-01 challenge fails — see
    README step 2. Set to false for the escape hatch: nginx serves plain
    HTTP only, for an operator putting their own TLS-terminating reverse
    proxy or CDN in front of this instance instead.
  EOT
  type        = bool
  default     = true
}

variable "letsencrypt_email" {
  description = <<-EOT
    Contact address Let's Encrypt sends expiry/revocation notices to.
    Required when enable_tls is true (Certbot's own --email flag); ignored
    when enable_tls is false.
  EOT
  type        = string
  default     = ""
  validation {
    condition     = !var.enable_tls || length(trimspace(var.letsencrypt_email)) > 0
    error_message = "letsencrypt_email must be set when enable_tls is true — Certbot requires a contact address."
  }
}

# ---- Database ---------------------------------------------------------------

variable "db_instance_class" {
  type    = string
  default = "db.t4g.micro"
}

variable "db_allocated_storage_gb" {
  type    = number
  default = 20
}

variable "db_engine_version" {
  description = "Postgres major/minor version. Must be a version RDS lists pgvector support for."
  type        = string
  default     = "16.4"
}

variable "db_backup_retention_days" {
  description = "Shorter than the full stack's default (7) on purpose — this is the light/small-deployment option, not the one asked to hold a long rollback window."
  type        = number
  default     = 3
}

variable "db_final_snapshot_generation" {
  description = <<-EOT
    Feeds rds.tf's random_id.final_snapshot as a keepers value, so a
    deliberate replacement gets a fresh final-snapshot suffix without
    deriving it from the resource being deleted (which would cycle). Bump
    this before destroying and recreating the RDS instance in the SAME
    state — otherwise the reused suffix collides with a snapshot an earlier
    deletion already left behind. ElastiCache has no equivalent here: this
    stack's replication group takes no final snapshot at all (elasticache.tf).
  EOT
  type        = number
  default     = 1
}

# ---- Redis ------------------------------------------------------------------

variable "redis_node_type" {
  type    = string
  default = "cache.t4g.micro"
}

# ---- Observability -----------------------------------------------------------

variable "log_retention_days" {
  type    = number
  default = 14
}

variable "ecr_tagged_image_retain_count" {
  description = "Same reasoning as the full stack's own variable — a generous rollback window under IMMUTABLE tags, not a tuned value. Must be a positive integer."
  type        = number
  default     = 10

  validation {
    condition     = var.ecr_tagged_image_retain_count >= 1 && floor(var.ecr_tagged_image_retain_count) == var.ecr_tagged_image_retain_count
    error_message = "ecr_tagged_image_retain_count must be a positive integer."
  }
}

variable "enable_deep_monitoring" {
  description = <<-EOT
    Toggles alarms.tf's SNS topic and the instance StatusCheckFailed alarm.
    Off by default — an operator who has not yet decided where alerts
    should go gets no half-wired SNS topic with nothing subscribed to it,
    same reasoning as the full stack's own toggle.
  EOT
  type        = bool
  default     = false
}
