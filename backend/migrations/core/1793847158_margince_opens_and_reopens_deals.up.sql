-- Margince may open a deal from buying evidence, and reopen a lost one, for
-- the company's owner. Both are corrections the owner can undo. Every existing
-- row already holds the wider check, so it is added without a scan; the next
-- migration validates it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal_correction
    DROP CONSTRAINT deal_correction_tier_check,
    ADD CONSTRAINT deal_correction_tier_check
        CHECK (correction = ANY (ARRAY['auto_apply'::text, 'provisional_confirm'::text,
                                       'downgrade_and_review'::text, 'close_lost'::text,
                                       'deal_close_date'::text, 'deal_amount'::text,
                                       'deal_probability'::text, 'stage_move'::text,
                                       'deal_open'::text, 'deal_revive'::text])) NOT VALID;
