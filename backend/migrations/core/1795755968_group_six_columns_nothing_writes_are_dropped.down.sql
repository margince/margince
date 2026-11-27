SET LOCAL lock_timeout = '3s';

ALTER TABLE contact_provider_claim ADD COLUMN validation_status text;

ALTER TABLE finance_invoice ADD COLUMN raw_status text;

ALTER TABLE assurance_run ADD COLUMN digest text;

ALTER TABLE approval_autonomy_policy
  ADD COLUMN promoted_at timestamp with time zone,
  ADD COLUMN demoted_at timestamp with time zone;
