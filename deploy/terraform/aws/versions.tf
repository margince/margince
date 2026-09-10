terraform {
  required_version = ">= 1.7.0"

  # No backend block: state defaults to local, which puts the RDS/Redis/
  # keyvault/webhook credentials every resource in secrets.tf generates into
  # a plaintext file on whatever machine runs `terraform apply` — fine for a
  # one-off `terraform plan` against this reference stack, not fine for an
  # actual deployment. Uncomment and fill in for anything beyond that:
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
      Project   = "margince"
      ManagedBy = "terraform"
    }
  }
}
