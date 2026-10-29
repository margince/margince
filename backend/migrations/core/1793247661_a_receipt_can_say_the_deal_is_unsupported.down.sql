SET LOCAL lock_timeout = '3s';

DELETE FROM assurance_deal_receipt WHERE outcome = 'unsupported';

ALTER TABLE assurance_deal_receipt
    DROP CONSTRAINT assurance_deal_receipt_outcome_check,
    ADD CONSTRAINT assurance_deal_receipt_outcome_check
        CHECK (outcome IN ('eligible', 'review_required', 'waiting', 'unknown'));
