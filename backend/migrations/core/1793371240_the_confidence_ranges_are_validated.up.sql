-- Validate the two ranges the migration before this one added NOT VALID.
--
-- Their own file because the runner wraps each file in one transaction, so the
-- scan runs under SHARE UPDATE EXCLUSIVE rather than behind the previous
-- file's ACCESS EXCLUSIVE.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_thread_verdict VALIDATE CONSTRAINT capture_thread_verdict_confidence_range;
ALTER TABLE ai_call VALIDATE CONSTRAINT ai_call_decision_confidence_range;
