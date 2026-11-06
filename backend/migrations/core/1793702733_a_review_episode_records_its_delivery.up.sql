-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Where a review episode's delivery stands, beside the episode and its task.
--
-- delivery_state is NULL for a finding that is not a review and for an episode
-- no pass has queued yet. delivered_assignee_id is the seat the system gave the
-- task, so a later owner change moves the task only while nobody chose another.
--
-- Nullable and defaulted columns only, so no row is rewritten.
SET LOCAL lock_timeout = '3s';

ALTER TABLE assurance_exception
    ADD COLUMN delivery_state text
        CONSTRAINT assurance_exception_delivery_state_check
        CHECK (delivery_state IN ('pending', 'failed', 'delivered')),
    ADD COLUMN delivery_attempts integer NOT NULL DEFAULT 0,
    ADD COLUMN delivered_assignee_id uuid;
