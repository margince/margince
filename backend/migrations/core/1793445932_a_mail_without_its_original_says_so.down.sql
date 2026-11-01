-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A mail with no original to check against reads as unassessed again, which is
-- the state it held before it could say so.
SET LOCAL lock_timeout = '5s';

UPDATE activity SET body_completeness = 'unassessed' WHERE body_completeness = 'unverifiable';
ALTER TABLE activity DROP CONSTRAINT activity_body_completeness_check;
ALTER TABLE activity
    ADD CONSTRAINT activity_body_completeness_check
        CHECK (body_completeness IN ('complete', 'partial', 'unreadable', 'forwarded', 'unassessed')) NOT VALID;
