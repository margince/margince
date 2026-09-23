terraform {
  required_version = ">= 1.7.0"

  # No backend block by default: state defaults to local, which writes every
  # generated credential from secrets.tf (RDS/Redis/keyvault/webhook/admin/
  # blobstore) into a plaintext file on the machine that runs `terraform
  # apply`. That is only acceptable for a one-off `terraform plan` against this
  # reference stack. Before running `terraform apply` for any real deployment,
  # uncomment and fill in the backend block below with a protected S3 bucket
  # that only authorized deployment identities can reach, and verify that
  # `terraform init -backend-config=...` (or the filled-in block) points at it.
  #
  # backend "s3" {
  #   bucket       = "your-terraform-state-bucket"
  #   key          = "margince/aws/terraform.tfstate"
  #   region       = "eu-central-1"
  #   encrypt      = true
  #   use_lockfile = true # S3's own native locking (Terraform >= 1.10); use
  #                       # a DynamoDB dynamodb_table instead on an older CLI
  # }
  #
  # No bucket name is filled in above on purpose — an operator's state bucket
  # is theirs to own and scope access to, the same reasoning
  # docs/deployment.md gives for keeping concrete deployment specifics out of
  # this repo.

  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "~> 5.60"
    }
    random = {
      source  = "hashicorp/random"
      version = "~> 3.6"
    }
  }
}

provider "aws" {
  region = var.aws_region

  default_tags {
    tags = {
      Project     = "margince"
      ManagedBy   = "terraform"
      Environment = var.environment
    }
  }
}
