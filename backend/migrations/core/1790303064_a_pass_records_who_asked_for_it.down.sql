SET LOCAL lock_timeout = '5s';

ALTER TABLE assurance_run DROP CONSTRAINT IF EXISTS assurance_run_requester_is_named;
ALTER TABLE assurance_run DROP COLUMN IF EXISTS requested_by;
