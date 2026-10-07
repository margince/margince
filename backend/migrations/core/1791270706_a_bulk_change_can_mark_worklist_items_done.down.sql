SET LOCAL lock_timeout = '5s';

-- Held before the check below, so no completion can commit between it and the
-- narrowed constraints.
LOCK TABLE bulk_operation IN ACCESS EXCLUSIVE MODE;
-- A recorded completion is history an undo reads, so the downgrade refuses
-- rather than deleting it.
DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM bulk_operation WHERE verb = 'complete' OR record_type = 'worklist_item') THEN
    RAISE EXCEPTION 'bulk_operation holds Worklist completions; this downgrade would have to delete them, so it refuses. Remove those rows deliberately first if losing them is intended'
      USING ERRCODE = 'check_violation';
  END IF;
END
$$;
ALTER TABLE bulk_operation DROP CONSTRAINT bulk_operation_verb_check;
ALTER TABLE bulk_operation ADD CONSTRAINT bulk_operation_verb_check
    CHECK (verb IN ('reassign_owner', 'archive', 'add_to_list', 'remove_from_list',
                    'add_tag', 'remove_tag', 'create_task')) NOT VALID;
ALTER TABLE bulk_operation DROP CONSTRAINT bulk_operation_record_type_check;
ALTER TABLE bulk_operation ADD CONSTRAINT bulk_operation_record_type_check
    CHECK (record_type IN ('contact', 'company', 'deal', 'lead')) NOT VALID;
