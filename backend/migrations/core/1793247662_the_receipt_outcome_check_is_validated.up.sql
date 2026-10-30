-- Validate the check the migration before this one added NOT VALID. Its own
-- file, so the scan runs under SHARE UPDATE EXCLUSIVE. It cannot fail: the
-- rule only got looser.
SET LOCAL lock_timeout = '3s';

ALTER TABLE assurance_deal_receipt VALIDATE CONSTRAINT assurance_deal_receipt_outcome_check;
