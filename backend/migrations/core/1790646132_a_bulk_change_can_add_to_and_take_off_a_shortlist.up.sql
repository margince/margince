-- A bulk change can add a selection to a Shortlist or take it off one. The
-- list it names rides params, beside a reassignment's owner.
SET LOCAL lock_timeout = '5s';

ALTER TABLE bulk_operation DROP CONSTRAINT bulk_operation_verb_check;
ALTER TABLE bulk_operation ADD CONSTRAINT bulk_operation_verb_check
    CHECK (verb IN ('reassign_owner', 'archive', 'add_to_list', 'remove_from_list'));
