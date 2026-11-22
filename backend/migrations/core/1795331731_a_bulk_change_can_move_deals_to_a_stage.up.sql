-- A bulk change can move deals to a stage. move_stage is a verb of the deal
-- record type. Added NOT VALID and validated in the file beside this one, so the
-- scan does not run under the ACCESS EXCLUSIVE this ALTER takes.
SET LOCAL lock_timeout = '5s';

ALTER TABLE bulk_operation DROP CONSTRAINT bulk_operation_verb_check;
ALTER TABLE bulk_operation ADD CONSTRAINT bulk_operation_verb_check
    CHECK (verb IN ('reassign_owner', 'archive', 'add_to_list', 'remove_from_list',
                    'add_tag', 'remove_tag', 'create_task', 'move_stage', 'complete')) NOT VALID;
