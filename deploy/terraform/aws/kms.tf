# One customer-managed key for everything this stack stores at rest: RDS,
# ElastiCache, S3, EFS, Secrets Manager, ECR. A CMK over the AWS-managed
# defaults buys two things a managed key cannot: a key-usage trail in
# CloudTrail (who decrypted what, when) and the ability to disable or
# schedule deletion of the key to cut off every one of these services at
# once, independent of anything IAM controls.
#
# One key, not six: this stack's blast-radius boundary is already the IAM
# grants below (only the ECS execution role and the blobstore IAM user can
# use it, and only for the specific actions each needs), so a second key per
# service would multiply the grants to manage without narrowing anything a
# compromised execution role could already reach through the secrets it
# holds. Split later if a real separation-of-duty requirement asks for it.
#
# No custom key policy: the default AWS applies (full access to the account
# root) delegates entirely to IAM, which is what lets the targeted grants
# below — and nothing else — actually use the key.
resource "aws_kms_key" "data" {
  description             = "${var.name_prefix} — CMK for RDS/ElastiCache/S3/EFS/Secrets Manager/ECR at rest"
  enable_key_rotation     = true
  deletion_window_in_days = 30
}

resource "aws_kms_alias" "data" {
  name          = "alias/${var.name_prefix}-data"
  target_key_id = aws_kms_key.data.key_id
}
