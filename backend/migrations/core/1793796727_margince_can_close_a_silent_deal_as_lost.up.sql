-- Margince closes a deal the customer stopped answering as lost, and records
-- that as a correction its owner can undo. The undo reopens the deal through
-- the stage move the close wrote, so the correction names that history row.
-- Every existing row already holds the wider check, so it is added without a
-- scan; the next migration validates it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal_correction
    ADD COLUMN stage_history_id uuid REFERENCES deal_stage_history (id) ON DELETE SET NULL,
    DROP CONSTRAINT deal_correction_tier_check,
    ADD CONSTRAINT deal_correction_tier_check
        CHECK (correction = ANY (ARRAY['auto_apply'::text, 'provisional_confirm'::text,
                                       'downgrade_and_review'::text, 'close_lost'::text])) NOT VALID;

CREATE INDEX idx_deal_correction_stage_history ON deal_correction (stage_history_id);
