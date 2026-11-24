-- The constraints added NOT VALID by the file beside this one are validated here, in
-- their own transaction, so the scan runs under SHARE UPDATE EXCLUSIVE.
SET LOCAL lock_timeout = '5s';

ALTER TABLE deal_suggestion_evidence VALIDATE CONSTRAINT deal_suggestion_evidence_shape;
ALTER TABLE deal_suggestion_evidence VALIDATE CONSTRAINT deal_suggestion_evidence_activity_id_fkey;
ALTER TABLE deal_suggestion_evidence VALIDATE CONSTRAINT deal_suggestion_evidence_attachment_id_fkey;
ALTER TABLE deal_suggestion_evidence VALIDATE CONSTRAINT deal_suggestion_evidence_signal_id_fkey;
