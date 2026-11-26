-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM web_form WHERE handling_cutover_at IS NOT NULL) THEN
        RAISE EXCEPTION 'Cannot remove inquiry policy history after a website handling cutover';
    END IF;
END $$;

DROP INDEX web_form_handling_automation;
ALTER TABLE web_form DROP COLUMN handling_cutover_at, DROP COLUMN handling_automation_id;
