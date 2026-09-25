-- Validate the two checks the migration before this one widened NOT VALID.
--
-- Its own file because the runner wraps each file in ONE transaction: run in
-- the same one, the scan would sit under the ACCESS EXCLUSIVE the ALTER holds
-- to commit. Here the scan runs under SHARE UPDATE EXCLUSIVE, so ai_call writes
-- do not queue behind it.
--
-- It cannot fail: every row predating the widening satisfied the narrower set.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_call VALIDATE CONSTRAINT ai_call_kind_check;
ALTER TABLE ai_model_rate VALIDATE CONSTRAINT ai_model_rate_lane_check;
