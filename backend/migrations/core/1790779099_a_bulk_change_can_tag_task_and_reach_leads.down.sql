SET LOCAL lock_timeout = '5s';

DELETE FROM bulk_operation
 WHERE verb IN ('add_tag', 'remove_tag', 'create_task') OR record_type = 'lead';
ALTER TABLE bulk_operation DROP CONSTRAINT bulk_operation_verb_check;
ALTER TABLE bulk_operation ADD CONSTRAINT bulk_operation_verb_check
    CHECK (verb IN ('reassign_owner', 'archive', 'add_to_list', 'remove_from_list'));
ALTER TABLE bulk_operation DROP CONSTRAINT bulk_operation_record_type_check;
ALTER TABLE bulk_operation ADD CONSTRAINT bulk_operation_record_type_check
    CHECK (record_type IN ('contact', 'company', 'deal'));
