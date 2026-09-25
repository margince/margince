-- Validate the two checks the migration before this one added NOT VALID.
--
-- Its own file because the runner wraps each file in ONE transaction: run in
-- the same one, the scan would sit under the ACCESS EXCLUSIVE the ALTER holds
-- to commit. Here the scan runs under SHARE UPDATE EXCLUSIVE, so ai_call writes
-- do not queue behind it.
--
-- It cannot fail: every row predating the columns holds NULL in both.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_call VALIDATE CONSTRAINT ai_call_decision_answer_shape;
ALTER TABLE ai_call VALIDATE CONSTRAINT ai_call_decision_answer_kind;
