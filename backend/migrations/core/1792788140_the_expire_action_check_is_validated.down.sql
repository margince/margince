-- Put the constraint back present and unvalidated, as the migration before this
-- one left it. Postgres has no ALTER that un-validates, so it is dropped and
-- re-added NOT VALID, which scans nothing.
SET LOCAL lock_timeout = '3s';

ALTER TABLE retention_policy
  DROP CONSTRAINT retention_policy_action_check,
  ADD CONSTRAINT retention_policy_action_check
    CHECK (action IN ('archive', 'anonymize', 'erase', 'expire')) NOT VALID;
