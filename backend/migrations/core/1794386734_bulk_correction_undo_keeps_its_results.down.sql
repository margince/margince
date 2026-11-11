-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM bulk_operation WHERE verb = 'undo_corrections') THEN
        RAISE EXCEPTION 'Cannot discard saved correction Undo results';
    END IF;
END $$;

ALTER TABLE bulk_operation DROP CONSTRAINT bulk_operation_verb_check;
ALTER TABLE bulk_operation ADD CONSTRAINT bulk_operation_verb_check
    CHECK (verb IN ('reassign_owner', 'archive', 'add_to_list', 'remove_from_list', 'add_tag', 'remove_tag', 'create_task', 'complete'));
