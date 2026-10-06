SET LOCAL lock_timeout = '5s';

DELETE FROM bulk_operation WHERE verb = 'complete' OR record_type = 'worklist_item';
ALTER TABLE bulk_operation DROP CONSTRAINT bulk_operation_verb_check;
ALTER TABLE bulk_operation ADD CONSTRAINT bulk_operation_verb_check
    CHECK (verb IN ('reassign_owner', 'archive', 'add_to_list', 'remove_from_list',
                    'add_tag', 'remove_tag', 'create_task')) NOT VALID;
ALTER TABLE bulk_operation DROP CONSTRAINT bulk_operation_record_type_check;
ALTER TABLE bulk_operation ADD CONSTRAINT bulk_operation_record_type_check
    CHECK (record_type IN ('contact', 'company', 'deal', 'lead')) NOT VALID;
