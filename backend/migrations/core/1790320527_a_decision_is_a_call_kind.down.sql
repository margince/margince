-- Put the narrow checks back, NOT VALID.
--
-- Deleting the decision rows to make a validated check fit would destroy trace
-- the operator may still need, so the narrow checks return unvalidated: new
-- rows are held to them, and any decision row written while this migration
-- stood stays in place, unchecked.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_call
  DROP CONSTRAINT ai_call_kind_check,
  ADD CONSTRAINT ai_call_kind_check
    CHECK (kind = ANY (ARRAY['completion'::text, 'embedding'::text])) NOT VALID;

ALTER TABLE ai_model_rate
  DROP CONSTRAINT ai_model_rate_lane_check,
  ADD CONSTRAINT ai_model_rate_lane_check
    CHECK (lane = ANY (ARRAY['chat'::text, 'embeddings'::text])) NOT VALID;
