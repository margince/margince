-- A decision-model call is traced like any other attempt, under its own kind,
-- and its rate is filed under its own lane.
--
-- Added NOT VALID: every existing row already satisfies the wider set, and the
-- scan runs in the next migration's own transaction, under a lock that lets
-- ai_call writes through.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_call
  DROP CONSTRAINT ai_call_kind_check,
  ADD CONSTRAINT ai_call_kind_check
    CHECK (kind = ANY (ARRAY['completion'::text, 'embedding'::text, 'decision'::text])) NOT VALID;

ALTER TABLE ai_model_rate
  DROP CONSTRAINT ai_model_rate_lane_check,
  ADD CONSTRAINT ai_model_rate_lane_check
    CHECK (lane = ANY (ARRAY['chat'::text, 'embeddings'::text, 'decisions'::text])) NOT VALID;
