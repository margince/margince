-- Margince reads each open deal at night and may set its close date, amount
-- and probability, each one a correction its owner can undo. The deal's
-- receipt keeps the last verdict, so a deal whose evidence has not moved is
-- not read again. A deal the input check has not walked yet gets a receipt
-- from the verdict alone, which names no run.
-- Every existing row already holds the wider check, so it is added without a
-- scan; the next migration validates it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE assurance_deal_receipt
    ALTER COLUMN run_id DROP NOT NULL,
    ADD COLUMN verdict_input_checksum text,
    ADD COLUMN verdict_basis_digest text,
    ADD COLUMN verdict_prompt_version text,
    ADD COLUMN verdict_assessed_at timestamptz,
    ADD COLUMN verdict_next_due_at timestamptz,
    ADD COLUMN verdict_actions jsonb;

ALTER TABLE deal_correction
    DROP CONSTRAINT deal_correction_tier_check,
    ADD CONSTRAINT deal_correction_tier_check
        CHECK (correction = ANY (ARRAY['auto_apply'::text, 'provisional_confirm'::text,
                                       'downgrade_and_review'::text, 'close_lost'::text,
                                       'deal_close_date'::text, 'deal_amount'::text,
                                       'deal_probability'::text])) NOT VALID;
