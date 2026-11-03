-- Put the check back present and unvalidated, as this migration found it.
-- Postgres has no ALTER that un-validates, so it is dropped and re-added.
SET LOCAL lock_timeout = '3s';

ALTER TABLE assurance_deal_receipt
    DROP CONSTRAINT assurance_deal_receipt_outcome_check,
    ADD CONSTRAINT assurance_deal_receipt_outcome_check
        CHECK (outcome IN ('eligible', 'review_required', 'unsupported', 'waiting', 'unknown')) NOT VALID;
