-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '5s';

-- Back to the schema that could route a review to a card. Every review comes
-- back with no card named; the audit log still records each routing.
ALTER TABLE communication_review ADD COLUMN approval_id uuid;

-- NOT VALID, as the up migration's CHECK was. Both only widen what the rows
-- already hold, so nothing here could fail.
ALTER TABLE communication_review
    DROP CONSTRAINT communication_review_state,
    ADD CONSTRAINT communication_review_state CHECK (state = ANY (ARRAY[
        'needs_context'::text, 'needs_repair'::text, 'awaiting_decision'::text,
        'resolved'::text, 'superseded'::text, 'cancelled'::text])) NOT VALID,
    ADD CONSTRAINT communication_review_routing_shape CHECK (
        (state <> 'awaiting_decision'::text) OR (approval_id IS NOT NULL)) NOT VALID;

CREATE INDEX communication_review_awaiting
    ON communication_review (opened_at)
    WHERE state = 'awaiting_decision';
