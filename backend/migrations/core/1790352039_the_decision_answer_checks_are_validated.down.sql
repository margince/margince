-- Put both checks back the way this migration found them: unvalidated.
--
-- Postgres has no ALTER that un-validates, so each is dropped and re-added NOT
-- VALID, which takes ACCESS EXCLUSIVE without a scan.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_call
  DROP CONSTRAINT ai_call_decision_answer_shape,
  ADD CONSTRAINT ai_call_decision_answer_shape
    CHECK ((decision_choice IS NULL) = (decision_confidence IS NULL)) NOT VALID,
  DROP CONSTRAINT ai_call_decision_answer_kind,
  ADD CONSTRAINT ai_call_decision_answer_kind
    CHECK (decision_choice IS NULL OR kind = 'decision') NOT VALID;
