-- Validate the approval effect-failure pairing.
--
-- effect_failed_at and effect_failure are one fact in two columns: a time with
-- no reason, or a reason with no time, is a failure the record cannot explain.
-- The rule has bound new and changed rows since it was added and was never
-- checked against the rows already there.
--
-- Its own file so the scan runs under SHARE UPDATE EXCLUSIVE rather than under
-- the ACCESS EXCLUSIVE the adding migration held until it committed.
SET LOCAL lock_timeout = '3s';

ALTER TABLE approval VALIDATE CONSTRAINT approval_effect_failure_is_stated;
