-- Validate the check the migration before this one added NOT VALID.
--
-- Its own file because the runner wraps each file in ONE transaction: run in
-- the same one, this scan would sit under the ACCESS EXCLUSIVE that ALTER holds
-- to commit. Here it is the only statement in its own transaction, so the scan
-- runs under SHARE UPDATE EXCLUSIVE and ai_call writes do not queue behind it.
--
-- It cannot fail: the column was created in that migration with a constant
-- default, so every row it scans carries ''.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_call VALIDATE CONSTRAINT ai_call_schema_downgrade_check;
