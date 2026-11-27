SET LOCAL lock_timeout = '3s';

-- No production statement reads or writes these columns, and no contract or
-- extension type names them.
ALTER TABLE approval_autonomy_policy
  DROP COLUMN promoted_at,
  DROP COLUMN demoted_at;

ALTER TABLE assurance_run DROP COLUMN digest;

ALTER TABLE finance_invoice DROP COLUMN raw_status;

ALTER TABLE contact_provider_claim DROP COLUMN validation_status;
