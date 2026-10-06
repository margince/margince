-- A bulk change can mark Worklist tasks and promises done. A worklist_item is a
-- task or a conversation claim; complete is the only verb it takes.
-- Added NOT VALID and validated in the file beside this one, so the scan does
-- not run under the ACCESS EXCLUSIVE this ALTER takes.
SET LOCAL lock_timeout = '5s';

ALTER TABLE bulk_operation DROP CONSTRAINT bulk_operation_verb_check;
ALTER TABLE bulk_operation ADD CONSTRAINT bulk_operation_verb_check
    CHECK (verb IN ('reassign_owner', 'archive', 'add_to_list', 'remove_from_list',
                    'add_tag', 'remove_tag', 'create_task', 'complete')) NOT VALID;

ALTER TABLE bulk_operation DROP CONSTRAINT bulk_operation_record_type_check;
ALTER TABLE bulk_operation ADD CONSTRAINT bulk_operation_record_type_check
    CHECK (record_type IN ('contact', 'company', 'deal', 'lead', 'worklist_item')) NOT VALID;
