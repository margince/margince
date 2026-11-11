-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

ALTER TABLE web_form
    ADD COLUMN handling_automation_id uuid REFERENCES automation(id) ON DELETE RESTRICT,
    ADD COLUMN handling_cutover_at timestamptz;

CREATE UNIQUE INDEX web_form_handling_automation
    ON web_form (handling_automation_id)
    WHERE handling_automation_id IS NOT NULL;
