SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_call
  DROP CONSTRAINT ai_call_decision_answer_kind,
  DROP CONSTRAINT ai_call_decision_answer_shape,
  DROP COLUMN decision_confidence,
  DROP COLUMN decision_choice;
