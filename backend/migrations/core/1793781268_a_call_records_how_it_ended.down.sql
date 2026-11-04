-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Calls no longer record how they ended.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity
    DROP CONSTRAINT IF EXISTS activity_call_outcome_kind_check,
    DROP CONSTRAINT IF EXISTS activity_call_outcome_check,
    DROP COLUMN IF EXISTS call_outcome;
