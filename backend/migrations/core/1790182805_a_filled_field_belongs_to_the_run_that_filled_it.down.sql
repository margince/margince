SET LOCAL lock_timeout = '5s';

ALTER TABLE provider_applied_field
    DROP CONSTRAINT IF EXISTS provider_applied_field_run_subject_fkey;
ALTER TABLE provider_run DROP CONSTRAINT IF EXISTS provider_run_subject_identity;
