-- Put the check back present and unvalidated. Postgres has no ALTER that
-- un-validates, so it is dropped and re-added NOT VALID, without a scan.
SET LOCAL lock_timeout = '3s';

ALTER TABLE audit_log
    DROP CONSTRAINT audit_log_images_are_absent_or_present,
    ADD CONSTRAINT audit_log_images_are_absent_or_present CHECK (
        before <> 'null'::jsonb AND after <> 'null'::jsonb) NOT VALID;
