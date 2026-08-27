-- What a priced model is FOR: the chat tiers, or the embeddings lane.
--
-- The routing form offers an installation the models it can price, and it has
-- nothing else to tell a chat model from an embedder — a zero output price is
-- every local chat row too. Existing rows default to 'chat', which is what the
-- overwhelming majority are; an operator whose sheet already carries an embedder
-- corrects that one row the same way they correct a price.
SET LOCAL lock_timeout = '3s';

ALTER TABLE ai_model_rate
    ADD COLUMN lane text NOT NULL DEFAULT 'chat';

ALTER TABLE ai_model_rate
    ADD CONSTRAINT ai_model_rate_lane_check CHECK ((lane = ANY (ARRAY['chat'::text, 'embeddings'::text])));
