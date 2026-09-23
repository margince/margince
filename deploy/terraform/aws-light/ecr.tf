data "aws_caller_identity" "current" {}

# IMMUTABLE, same as the full stack (deploy/terraform/aws/ecs.tf) and for
# the same reason: a pushed tag can never be silently overwritten, which is
# what makes var.image_tag's "no floating latest" rule actually enforceable.
# No customer-managed KMS key in this stack (see kms's absence, noted in
# rds.tf/elasticache.tf/s3.tf) — these use the AWS-managed aws/ecr default
# key instead, one less credential this stack's single instance role needs
# a KMS grant for.
resource "aws_ecr_repository" "api" {
  name                 = "${var.name_prefix}/api"
  image_tag_mutability = "IMMUTABLE"
  image_scanning_configuration { scan_on_push = true }
  tags = { Name = "${var.name_prefix}-api", Component = "container-registry" }
}

resource "aws_ecr_repository" "worker" {
  name                 = "${var.name_prefix}/worker"
  image_tag_mutability = "IMMUTABLE"
  image_scanning_configuration { scan_on_push = true }
  tags = { Name = "${var.name_prefix}-worker", Component = "container-registry" }
}

resource "aws_ecr_repository" "web" {
  name                 = "${var.name_prefix}/web"
  image_tag_mutability = "IMMUTABLE"
  image_scanning_configuration { scan_on_push = true }
  tags = { Name = "${var.name_prefix}-web", Component = "container-registry" }
}

# Same two-rule lifecycle policy as the full stack, same reasoning: rule 1
# clears untagged digests left behind when IMMUTABLE forces a tag move to a
# new push; rule 2 caps how many tagged (released) images pile up forever,
# since IMMUTABLE means none of them are ever reclaimed by a later push to
# the same tag.
locals {
  untagged_expiry_policy = jsonencode({
    rules = [
      {
        rulePriority = 1
        description  = "Expire untagged images after 14 days"
        selection = {
          tagStatus   = "untagged"
          countType   = "sinceImagePushed"
          countUnit   = "days"
          countNumber = 14
        }
        action = { type = "expire" }
      },
      {
        rulePriority = 2
        description  = "Keep only the most recent ${var.ecr_tagged_image_retain_count} tagged (released) images"
        selection = {
          tagStatus      = "tagged"
          tagPatternList = ["*"]
          countType      = "imageCountMoreThan"
          countNumber    = var.ecr_tagged_image_retain_count
        }
        action = { type = "expire" }
      },
    ]
  })
}

resource "aws_ecr_lifecycle_policy" "api" {
  repository = aws_ecr_repository.api.name
  policy     = local.untagged_expiry_policy
}

resource "aws_ecr_lifecycle_policy" "worker" {
  repository = aws_ecr_repository.worker.name
  policy     = local.untagged_expiry_policy
}

resource "aws_ecr_lifecycle_policy" "web" {
  repository = aws_ecr_repository.web.name
  policy     = local.untagged_expiry_policy
}
