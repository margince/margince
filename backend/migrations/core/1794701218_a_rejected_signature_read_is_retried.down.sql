-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

DROP INDEX IF EXISTS idx_contact_signature_enrich_state_retry_activity;
ALTER TABLE contact_signature_enrich_state
    DROP COLUMN IF EXISTS retry_count,
    DROP COLUMN IF EXISTS retry_activity_id,
    DROP COLUMN IF EXISTS last_skip_reason,
    DROP COLUMN IF EXISTS last_outcome;
