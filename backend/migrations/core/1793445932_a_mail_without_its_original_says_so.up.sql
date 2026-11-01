-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- A captured mail whose stored original is gone, erased or no longer decodes
-- has nothing left to check its text against. It says so, rather than reading
-- as a check that is still to come.
SET LOCAL lock_timeout = '5s';

ALTER TABLE activity DROP CONSTRAINT activity_body_completeness_check;
ALTER TABLE activity
    ADD CONSTRAINT activity_body_completeness_check
        CHECK (body_completeness IN ('complete', 'partial', 'unreadable', 'forwarded', 'unassessed', 'unverifiable')) NOT VALID;
