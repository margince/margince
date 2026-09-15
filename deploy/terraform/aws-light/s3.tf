# The blobstore client (backend/internal/platform/blobstore/s3.go) is a
# generic minio-go client authenticating with static keys — same reasoning
# as the full stack's own s3.tf for why this is an IAM user's access key,
# not the instance role, that satisfies MARGINCE_BLOBSTORE_ACCESS_KEY/
# SECRET_KEY.
#
# This bucket also holds the one config object the full stack instead
# mounts via EFS: ${var.name_prefix}/margince.yaml — see ec2.tf's user-data,
# which fetches it at boot. No EFS in this stack: one instance has local
# disk, and EFS's entire value (shared mount across many tasks) buys
# nothing when there is only ever one.

resource "aws_s3_bucket" "blobstore" {
  bucket = "${var.name_prefix}-blobstore"
  tags   = { Name = "${var.name_prefix}-blobstore", Component = "storage" }

  lifecycle {
    prevent_destroy = true
  }
}

resource "aws_s3_bucket_ownership_controls" "blobstore" {
  bucket = aws_s3_bucket.blobstore.id
  rule {
    object_ownership = "BucketOwnerEnforced"
  }
}

resource "aws_s3_bucket_public_access_block" "blobstore" {
  bucket                  = aws_s3_bucket.blobstore.id
  block_public_acls       = true
  block_public_policy     = true
  ignore_public_acls      = true
  restrict_public_buckets = true
}

# SSE-S3 (AES256), not SSE-KMS: the full stack's s3.tf uses its own CMK for
# this; "light" has no customer-managed key to reach for (kms.tf doesn't
# exist here), and AES256 is still real encryption at rest, just under a
# key this stack doesn't have to grant or rotate.
resource "aws_s3_bucket_server_side_encryption_configuration" "blobstore" {
  bucket = aws_s3_bucket.blobstore.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm = "AES256"
    }
  }
}

resource "aws_s3_bucket_lifecycle_configuration" "blobstore" {
  bucket = aws_s3_bucket.blobstore.id

  depends_on = [aws_s3_bucket_versioning.blobstore]

  rule {
    id     = "abort-incomplete-multipart-uploads"
    status = "Enabled"
    filter {}

    abort_incomplete_multipart_upload {
      days_after_initiation = 7
    }
  }

  rule {
    id     = "expire-noncurrent-versions"
    status = "Enabled"
    filter {}

    noncurrent_version_expiration {
      noncurrent_days = 90
    }
  }
}

resource "aws_s3_bucket_versioning" "blobstore" {
  bucket = aws_s3_bucket.blobstore.id
  versioning_configuration {
    status = "Enabled"
  }
}

# Server-side backstop, same as the full stack: MARGINCE_BLOBSTORE_USE_SSL
# makes the CLIENT ask for TLS, this makes S3 itself refuse a request that
# didn't arrive over it, regardless of what the client meant to do. This is
# a security floor kept even though the KMS-specific deny statements
# (DenyWrongEncryption/DenyWrongKMSKey) aren't — those exist only because
# the full stack forces a specific CMK; SSE-S3 here has no key ID to check
# against.
resource "aws_s3_bucket_policy" "blobstore_tls_only" {
  bucket = aws_s3_bucket.blobstore.id

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid       = "DenyInsecureTransport"
        Effect    = "Deny"
        Principal = "*"
        Action    = "s3:*"
        Resource = [
          aws_s3_bucket.blobstore.arn,
          "${aws_s3_bucket.blobstore.arn}/*",
        ]
        Condition = {
          Bool = { "aws:SecureTransport" = "false" }
        }
      },
    ]
  })
}

resource "aws_iam_user" "blobstore" {
  name = "${var.name_prefix}-blobstore"
  tags = { Name = "${var.name_prefix}-blobstore", Component = "security" }
}

resource "aws_iam_access_key" "blobstore" {
  user = aws_iam_user.blobstore.name
}

resource "aws_iam_user_policy" "blobstore" {
  name = "${var.name_prefix}-blobstore-bucket-only"
  user = aws_iam_user.blobstore.name

  policy = jsonencode({
    Version = "2012-10-17"
    Statement = [
      {
        Sid      = "ListOwnBucket"
        Effect   = "Allow"
        Action   = ["s3:ListBucket"]
        Resource = [aws_s3_bucket.blobstore.arn]
      },
      {
        Sid      = "ReadWriteObjects"
        Effect   = "Allow"
        Action   = ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"]
        Resource = ["${aws_s3_bucket.blobstore.arn}/*"]
      },
      # This credential is handed to the api/worker containers for
      # attachment storage (arbitrary caller-supplied keys — the blobstore
      # client has no built-in prefix restriction of its own). The instance
      # role's own ReadConfigObject grant (iam.tf) is the ONLY intended
      # reader of config/margince.yaml, and neither role has any legitimate
      # reason to write or delete it — an explicit Deny here closes the gap
      # a bug in attachment-key handling (or a leaked blobstore key) would
      # otherwise leave open to overwrite the boot config this same stack
      # fetches at every instance start.
      {
        Sid      = "DenyConfigObjectAccess"
        Effect   = "Deny"
        Action   = ["s3:GetObject", "s3:PutObject", "s3:DeleteObject"]
        Resource = ["${aws_s3_bucket.blobstore.arn}/config/*"]
      },
    ]
  })
}
