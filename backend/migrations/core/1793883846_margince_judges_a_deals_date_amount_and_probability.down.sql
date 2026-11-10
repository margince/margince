-- Rows a verdict wrote stay; the narrower check holds new rows only. A receipt
-- that names no run cannot keep the NOT NULL, so those receipts go.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal_correction
    DROP CONSTRAINT deal_correction_tier_check,
    ADD CONSTRAINT deal_correction_tier_check
        CHECK (correction = ANY (ARRAY['auto_apply'::text, 'provisional_confirm'::text,
                                       'downgrade_and_review'::text, 'close_lost'::text])) NOT VALID;

DELETE FROM assurance_deal_receipt WHERE run_id IS NULL;

ALTER TABLE assurance_deal_receipt
    DROP COLUMN verdict_actions,
    DROP COLUMN verdict_next_due_at,
    DROP COLUMN verdict_assessed_at,
    DROP COLUMN verdict_prompt_version,
    DROP COLUMN verdict_basis_digest,
    DROP COLUMN verdict_input_checksum,
    ALTER COLUMN run_id SET NOT NULL;
