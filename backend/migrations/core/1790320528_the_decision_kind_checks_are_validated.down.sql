-- Put both checks back the way this migration found them: widened, unvalidated.
--
-- Postgres has no ALTER that un-validates, so each is dropped and re-added NOT
-- VALID, which takes ACCESS EXCLUSIVE without a scan. The rows still satisfy
-- them; what is restored is the catalog's record of whether anybody has checked.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_call
  DROP CONSTRAINT ai_call_kind_check,
  ADD CONSTRAINT ai_call_kind_check
    CHECK (kind = ANY (ARRAY['completion'::text, 'embedding'::text, 'decision'::text])) NOT VALID;

ALTER TABLE ai_model_rate
  DROP CONSTRAINT ai_model_rate_lane_check,
  ADD CONSTRAINT ai_model_rate_lane_check
    CHECK (lane = ANY (ARRAY['chat'::text, 'embeddings'::text, 'decisions'::text])) NOT VALID;
