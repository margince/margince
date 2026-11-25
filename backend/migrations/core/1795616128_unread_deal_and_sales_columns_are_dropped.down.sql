-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The columns come back with their types, defaults, checks, links and indexes.
-- What they held is not restored: a returning row carries the default or a blank.
SET LOCAL lock_timeout = '3s';

ALTER TABLE sales_initiative_baseline
    ADD COLUMN IF NOT EXISTS amount_minor_base bigint
        CONSTRAINT sales_initiative_baseline_amount_exact CHECK (((amount_minor_base >= '-9007199254740991'::bigint) AND (amount_minor_base <= '9007199254740991'::bigint))),
    ADD COLUMN IF NOT EXISTS captured_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS stage_id uuid,
    ADD COLUMN IF NOT EXISTS status text NOT NULL DEFAULT '';
ALTER TABLE sales_initiative_baseline ALTER COLUMN status DROP DEFAULT;

ALTER TABLE sales_initiative ADD COLUMN IF NOT EXISTS source text NOT NULL DEFAULT 'manual';
ALTER TABLE sales_initiative ALTER COLUMN source DROP DEFAULT;

ALTER TABLE sales_target_revision ADD COLUMN IF NOT EXISTS effective_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE sales_target_revision ALTER COLUMN effective_at DROP DEFAULT;

ALTER TABLE deal_product
    ADD COLUMN IF NOT EXISTS offer_line_item_id uuid
        CONSTRAINT deal_product_offer_line_item_id_fkey REFERENCES offer_line_item(id) ON DELETE SET NULL,
    ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now();
CREATE INDEX IF NOT EXISTS deal_product_offer_line_item_idx ON deal_product USING btree (offer_line_item_id)
    WHERE (offer_line_item_id IS NOT NULL);

ALTER TABLE activity_sales_state
    ADD COLUMN IF NOT EXISTS set_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS set_by text NOT NULL DEFAULT '';
ALTER TABLE activity_sales_state ALTER COLUMN set_by DROP DEFAULT;

ALTER TABLE deal_correction
    ADD COLUMN IF NOT EXISTS reversal_audit_id uuid
        CONSTRAINT deal_correction_reversal_audit_id_fkey REFERENCES audit_log(id) ON DELETE SET NULL;

ALTER TABLE deal_document_hide
    ADD COLUMN IF NOT EXISTS hidden_at timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS hidden_by text NOT NULL DEFAULT '';
ALTER TABLE deal_document_hide ALTER COLUMN hidden_by DROP DEFAULT;

ALTER TABLE deal_room_invitation
    ADD COLUMN IF NOT EXISTS failure_reason text,
    ADD COLUMN IF NOT EXISTS source text NOT NULL DEFAULT 'system';
ALTER TABLE deal_room_invitation ALTER COLUMN source DROP DEFAULT;

ALTER TABLE deal_room_session ADD COLUMN IF NOT EXISTS source text NOT NULL DEFAULT 'system';
ALTER TABLE deal_room_session ALTER COLUMN source DROP DEFAULT;

ALTER TABLE deal_stage_history ADD COLUMN IF NOT EXISTS fx_rate_at_change numeric;

ALTER TABLE deal_suggestion
    ADD COLUMN IF NOT EXISTS evidence_through timestamptz NOT NULL DEFAULT now(),
    ADD COLUMN IF NOT EXISTS decided_by uuid
        CONSTRAINT deal_suggestion_decided_by_fkey REFERENCES app_user(id) ON DELETE SET NULL;
ALTER TABLE deal_suggestion ALTER COLUMN evidence_through DROP DEFAULT;
CREATE INDEX IF NOT EXISTS idx_deal_suggestion_decided_by ON deal_suggestion USING btree (decided_by)
    WHERE (decided_by IS NOT NULL);

ALTER TABLE deal_loss_reason_mapping ADD COLUMN IF NOT EXISTS created_at timestamptz NOT NULL DEFAULT now();

ALTER TABLE deal_move_dismissal ADD COLUMN IF NOT EXISTS dismissed_at timestamptz NOT NULL DEFAULT now();
