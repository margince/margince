-- A decision attempt keeps the label it chose and the confidence it gave,
-- whether or not the answer stood. A below-floor answer is exactly the one a
-- floor is tuned from, and the payload that also holds it is kept only for the
-- terminal attempt and never for a no_payload task.
--
-- Nullable with no default, so adding them rewrites nothing. The two checks
-- are added NOT VALID and validated by the next migration, under a lock that
-- lets ai_call writes through.
--
-- double precision rather than real: the wire's confidence is a float64, and a
-- real would hand 0.62 back as 0.6200000047683716.
--
-- No range check on the confidence: the value is what the decision server
-- sent, and a trace row refused over a vendor's out-of-range number would lose
-- the whole logical call's trace to record nothing.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_call
  ADD COLUMN decision_choice text,
  ADD COLUMN decision_confidence double precision,
  ADD CONSTRAINT ai_call_decision_answer_shape
    CHECK ((decision_choice IS NULL) = (decision_confidence IS NULL)) NOT VALID,
  ADD CONSTRAINT ai_call_decision_answer_kind
    CHECK (decision_choice IS NULL OR kind = 'decision') NOT VALID;
