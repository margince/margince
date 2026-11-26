-- Validates the ai_usage floors the previous migration added. A day total that
-- a negative provider count lowered below zero is raised to zero first, the
-- value the meter now stores for such a count.
--
-- The ai_call floors stay NOT VALID: the table's history is unbounded, so a
-- correction of it is a data task. No writer updates an ai_call row, so an old
-- negative row refuses nothing. backend/gates/unvalidatedconstraints_test.go
-- carries the reason.
SET LOCAL lock_timeout = '3s';

UPDATE ai_usage
   SET calls = greatest(calls, 0), cached_hits = greatest(cached_hits, 0),
       tokens_in = greatest(tokens_in, 0), tokens_out = greatest(tokens_out, 0)
 WHERE calls < 0 OR cached_hits < 0 OR tokens_in < 0 OR tokens_out < 0;

ALTER TABLE ai_usage VALIDATE CONSTRAINT ai_usage_calls_not_negative;
ALTER TABLE ai_usage VALIDATE CONSTRAINT ai_usage_cached_hits_not_negative;
ALTER TABLE ai_usage VALIDATE CONSTRAINT ai_usage_tokens_in_not_negative;
ALTER TABLE ai_usage VALIDATE CONSTRAINT ai_usage_tokens_out_not_negative;
