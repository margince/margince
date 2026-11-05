-- Rows a stage move wrote stay; the narrower check holds new rows only.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal_correction
    DROP CONSTRAINT deal_correction_tier_check,
    ADD CONSTRAINT deal_correction_tier_check
        CHECK (correction = ANY (ARRAY['auto_apply'::text, 'provisional_confirm'::text,
                                       'downgrade_and_review'::text, 'close_lost'::text,
                                       'deal_close_date'::text, 'deal_amount'::text,
                                       'deal_probability'::text])) NOT VALID;
