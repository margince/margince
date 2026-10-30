-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A deal lost with a code and no note breaks the old rule, so the validation
-- fails while one exists rather than invent a note for it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal_stage_history DROP COLUMN lost_reason_code_at_change;

ALTER TABLE deal DROP CONSTRAINT deal_lost_reason;
ALTER TABLE deal ADD CONSTRAINT deal_lost_reason
    CHECK (status <> 'lost' OR lost_reason IS NOT NULL) NOT VALID;
ALTER TABLE deal VALIDATE CONSTRAINT deal_lost_reason;

ALTER TABLE deal
    DROP COLUMN lost_reason_mapping_id,
    DROP COLUMN lost_reason_basis,
    DROP COLUMN lost_reason_code;

DROP TABLE deal_loss_reason_mapping;
DROP TABLE deal_loss_reason;
