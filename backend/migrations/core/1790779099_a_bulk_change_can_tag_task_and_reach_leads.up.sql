-- A bulk change can put a tag on a selection or take it off, file a task under
-- each record, and act on leads. The tag and the task ride params, beside a
-- reassignment's owner and a list verb's Shortlist.
SET LOCAL lock_timeout = '5s';

ALTER TABLE bulk_operation DROP CONSTRAINT bulk_operation_verb_check;
ALTER TABLE bulk_operation ADD CONSTRAINT bulk_operation_verb_check
    CHECK (verb IN ('reassign_owner', 'archive', 'add_to_list', 'remove_from_list',
                    'add_tag', 'remove_tag', 'create_task'));

ALTER TABLE bulk_operation DROP CONSTRAINT bulk_operation_record_type_check;
ALTER TABLE bulk_operation ADD CONSTRAINT bulk_operation_record_type_check
    CHECK (record_type IN ('contact', 'company', 'deal', 'lead'));
