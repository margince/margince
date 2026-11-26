SET LOCAL lock_timeout = '3s';

-- No production statement writes these columns. A file's company is derived
-- from the record it is filed on, and no partner fit engine exists.
ALTER TABLE attachment DROP COLUMN company_id;

ALTER TABLE partner
  DROP COLUMN joined_at,
  DROP COLUMN renews_at,
  DROP COLUMN partner_fit_score,
  DROP COLUMN partner_fit_score_computed,
  DROP COLUMN partner_fit_override_reason;

COMMENT ON COLUMN app_user.seat_type IS
  'A licence tier: a read seat may read and never mutate. Nothing writes read yet, so every member is full.';

COMMENT ON FUNCTION weekly_plan_commitment_forget_manager() IS
  'Armed for a hard delete of a member, which the product does not perform: a member is deactivated, never deleted.';
