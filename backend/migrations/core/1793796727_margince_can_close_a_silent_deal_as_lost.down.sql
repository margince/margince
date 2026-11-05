-- Rows that record a close as lost stay; the narrower check holds new rows only.
SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS idx_deal_correction_stage_history;

ALTER TABLE deal_correction
    DROP COLUMN stage_history_id,
    DROP CONSTRAINT deal_correction_tier_check,
    ADD CONSTRAINT deal_correction_tier_check
        CHECK (correction = ANY (ARRAY['auto_apply'::text, 'provisional_confirm'::text,
                                       'downgrade_and_review'::text])) NOT VALID;
