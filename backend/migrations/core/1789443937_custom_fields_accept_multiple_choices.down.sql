-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Refuse rollback while multiselect definitions exist; never discard values.
SET LOCAL lock_timeout = '2s';
ALTER TABLE custom_field DROP CONSTRAINT custom_field_type_check;
ALTER TABLE custom_field ADD CONSTRAINT custom_field_type_check
 CHECK (type IN ('text', 'number', 'date', 'currency', 'picklist', 'boolean'));

-- Refuse to discard reviews containing selected choices.
DO $$ BEGIN
 IF EXISTS (SELECT 1 FROM activity_review_response WHERE choice_answers NOT IN ('{}'::jsonb, 'null'::jsonb)) THEN
  RAISE EXCEPTION 'Review choices exist; rollback would discard answers';
 END IF;
END $$;
ALTER TABLE activity_review_response DROP COLUMN choice_answers;
