-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

-- A read the pass rejects leaves the watermark and counts a retry against the
-- mail it rejected. At the cap the mail is consumed, and the reason stays here.
-- A newer mail starts its own count, so the count names its mail.
ALTER TABLE contact_signature_enrich_state
    ADD COLUMN last_outcome text
        CHECK (last_outcome IN ('applied', 'nothing_to_apply', 'skipped')),
    ADD COLUMN last_skip_reason text
        CHECK (last_skip_reason IN ('name_gate', 'evidence_gate', 'confidence', 'narrowed')),
    ADD COLUMN retry_activity_id uuid REFERENCES activity(id) ON DELETE SET NULL,
    ADD COLUMN retry_count smallint NOT NULL DEFAULT 0 CHECK (retry_count >= 0);

-- Partial, because the column is null on every read that was not rejected.
CREATE INDEX idx_contact_signature_enrich_state_retry_activity
    ON contact_signature_enrich_state (retry_activity_id)
    WHERE retry_activity_id IS NOT NULL;
