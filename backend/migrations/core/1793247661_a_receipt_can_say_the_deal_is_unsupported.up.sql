-- A deal the customer still talks about can lack a close date it backs or a
-- next step. The check leaves it out of the qualified forecast without raising
-- a review, and its receipt says so. Every existing row already holds the wider
-- check, so it is added without a scan.
SET LOCAL lock_timeout = '3s';

ALTER TABLE assurance_deal_receipt
    DROP CONSTRAINT assurance_deal_receipt_outcome_check,
    ADD CONSTRAINT assurance_deal_receipt_outcome_check
        CHECK (outcome IN ('eligible', 'review_required', 'unsupported', 'waiting', 'unknown')) NOT VALID;
