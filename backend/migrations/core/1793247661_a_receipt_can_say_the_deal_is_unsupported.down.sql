-- Rows written as unsupported stay; the narrower check holds new rows only.
SET LOCAL lock_timeout = '3s';

ALTER TABLE assurance_deal_receipt
    DROP CONSTRAINT assurance_deal_receipt_outcome_check,
    ADD CONSTRAINT assurance_deal_receipt_outcome_check
        CHECK (outcome IN ('eligible', 'review_required', 'waiting', 'unknown')) NOT VALID;
