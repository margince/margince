-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '2s';

DROP INDEX IF EXISTS ai_call_subject_idx;
ALTER TABLE ai_call
    DROP CONSTRAINT IF EXISTS ai_call_subject_shape,
    DROP COLUMN IF EXISTS subject_id,
    DROP COLUMN IF EXISTS subject_type;
