-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '2s';
ALTER TABLE custom_field DROP CONSTRAINT custom_field_type_check;
ALTER TABLE custom_field ADD CONSTRAINT custom_field_type_check
 CHECK (type IN ('text', 'number', 'date', 'currency', 'picklist', 'multiselect', 'boolean'));

ALTER TABLE activity_review_response ADD COLUMN choice_answers jsonb NOT NULL DEFAULT '{}'::jsonb;
