-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- No statement, trigger, export or erasure path read these columns. The check,
-- links and indexes that go with them named only columns dropped here.
SET LOCAL lock_timeout = '3s';

-- A baseline is read by its member deals alone.
ALTER TABLE sales_initiative_baseline DROP COLUMN IF EXISTS amount_minor_base,
    DROP COLUMN IF EXISTS captured_at,
    DROP COLUMN IF EXISTS stage_id,
    DROP COLUMN IF EXISTS status;

ALTER TABLE sales_initiative DROP COLUMN IF EXISTS source;

ALTER TABLE sales_target_revision DROP COLUMN IF EXISTS effective_at;

ALTER TABLE deal_product DROP COLUMN IF EXISTS offer_line_item_id,
    DROP COLUMN IF EXISTS created_at;

ALTER TABLE activity_sales_state DROP COLUMN IF EXISTS set_at,
    DROP COLUMN IF EXISTS set_by;

ALTER TABLE deal_correction DROP COLUMN IF EXISTS reversal_audit_id;

ALTER TABLE deal_document_hide DROP COLUMN IF EXISTS hidden_at,
    DROP COLUMN IF EXISTS hidden_by;

ALTER TABLE deal_room_invitation DROP COLUMN IF EXISTS failure_reason,
    DROP COLUMN IF EXISTS source;

ALTER TABLE deal_room_session DROP COLUMN IF EXISTS source;

ALTER TABLE deal_stage_history DROP COLUMN IF EXISTS fx_rate_at_change;

ALTER TABLE deal_suggestion DROP COLUMN IF EXISTS evidence_through,
    DROP COLUMN IF EXISTS decided_by;

ALTER TABLE deal_loss_reason_mapping DROP COLUMN IF EXISTS created_at;

ALTER TABLE deal_move_dismissal DROP COLUMN IF EXISTS dismissed_at;
