-- Put the check back present and unvalidated. Postgres has no ALTER that
-- un-validates, so it is dropped and re-added NOT VALID, without a scan.
SET LOCAL lock_timeout = '3s';

ALTER TABLE approval
    DROP CONSTRAINT approval_effect_failure_is_stated,
    ADD CONSTRAINT approval_effect_failure_is_stated CHECK (
        (effect_failed_at IS NULL) = (effect_failure IS NULL)) NOT VALID;
