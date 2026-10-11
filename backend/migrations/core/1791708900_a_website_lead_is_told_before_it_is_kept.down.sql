-- The ledger and the lead link carry foreign keys out to lead, approval and
-- comms_outbound; a bounded wait keeps the drop from stalling writes to them.
SET LOCAL lock_timeout = '3s';

DROP TABLE IF EXISTS lead_privacy_notice;
-- A lead's notice link cannot survive the column that names its subject.
DELETE FROM confirm_token WHERE lead_id IS NOT NULL;
ALTER TABLE confirm_token DROP CONSTRAINT IF EXISTS confirm_token_lead_is_only_told;
ALTER TABLE confirm_token DROP CONSTRAINT IF EXISTS confirm_token_one_subject;
DROP INDEX IF EXISTS ix_confirm_token_lead;
ALTER TABLE confirm_token DROP COLUMN IF EXISTS lead_id;
ALTER TABLE confirm_token ALTER COLUMN contact_id SET NOT NULL;
