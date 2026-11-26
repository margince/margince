SET LOCAL lock_timeout = '3s';

COMMENT ON FUNCTION weekly_plan_commitment_forget_manager() IS NULL;
COMMENT ON COLUMN app_user.seat_type IS NULL;

ALTER TABLE partner
  ADD COLUMN joined_at date,
  ADD COLUMN renews_at date,
  ADD COLUMN partner_fit_score smallint
    CONSTRAINT partner_partner_fit_score_check
    CHECK (((partner_fit_score IS NULL) OR ((partner_fit_score >= 0) AND (partner_fit_score <= 100)))),
  ADD COLUMN partner_fit_score_computed smallint
    CONSTRAINT partner_partner_fit_score_computed_check
    CHECK (((partner_fit_score_computed IS NULL) OR ((partner_fit_score_computed >= 0) AND (partner_fit_score_computed <= 100)))),
  ADD COLUMN partner_fit_override_reason text
    CONSTRAINT partner_partner_fit_override_reason_check
    CHECK (((partner_fit_override_reason IS NULL) OR (length(btrim(partner_fit_override_reason)) > 0)));

ALTER TABLE attachment ADD COLUMN company_id uuid;
