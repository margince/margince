# The blobstore client (backend/internal/platform/blobstore/s3.go) is a
# generic minio-go client authenticating with
# credentials.NewStaticV4(accessKey, secretKey) — it never reads the AWS SDK's
# default credential chain, so an IAM task role alone cannot authenticate it.
# A static, bucket-scoped IAM user's access key is therefore the only way to
# satisfy MARGINCE_BLOBSTORE_ACCESS_KEY/SECRET_KEY here, not a design choice
# this stack could avoid by preferring a role.

resource "aws_s3_bucket" "blobstore" {
  bucket = "${var.name_prefix}-blobstore"
}

# BucketOwnerEnforced disables ACLs entirely — every access decision runs
# through IAM/bucket policy alone, which is the only path this stack ever
# grants through anyway (the blobstore IAM user's policy below). New buckets
# default to this since April 2023, but explicit beats relying on a default
# an operator reading this file has no way to see.
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

resource "aws_s3_bucket_server_side_encryption_configuration" "blobstore" {
  bucket = aws_s3_bucket.blobstore.id
  rule {
    apply_server_side_encryption_by_default {
      sse_algorithm     = "aws:kms"
      kms_master_key_id = aws_kms_key.data.arn
    }
    # Bucket Keys cut the per-object KMS API calls minio-go's GetObject/
    # PutObject would otherwise make one-for-one, at no loss of security —
    # the data key is still unique per object, only the KMS round-trip to
    # mint it is amortized.
    bucket_key_enabled = true
  }
}

resource "aws_s3_bucket_versioning" "blobstore" {
  bucket = aws_s3_bucket.blobstore.id
  versioning_configuration {
    status = "Disabled"
  }
}

# MARGINCE_BLOBSTORE_USE_SSL=true (ecs.tf) makes the CLIENT ask for TLS; it
# does nothing to stop a plaintext request if that setting ever regressed.
# This is the server-side backstop — S3 itself will refuse any request that
# didn't arrive over TLS, independent of what the client meant to do.
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
      {
        # SSE-KMS checks the caller's KMS permissions in addition to its S3
        # permissions — the bucket-scoped S3 policy above says nothing about
        # whether this user may use the key every object in it is now
        # encrypted under. Without this, every GetObject/PutObject the
        # blobstore client makes fails.
        Sid      = "UseDataKey"
        Effect   = "Allow"
        Action   = ["kms:GenerateDataKey", "kms:Decrypt"]
        Resource = [aws_kms_key.data.arn]
      },
    ]
  })
}
