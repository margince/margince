SET LOCAL lock_timeout = '3s';
-- Where a price came from. The sync re-prices only what it or the seed wrote,
-- so a writer that names nothing is read as a person's price and left alone.
ALTER TABLE ai_model_rate ADD COLUMN source text NOT NULL DEFAULT 'manual'
  CONSTRAINT ai_model_rate_source_check CHECK (source IN ('manual', 'catalogue', 'seed'));
-- A price a person wrote carries a human on its audit trail. The seed writes
-- none and the Vertex copy is the migration's, so every other row is the seed's.
UPDATE ai_model_rate r SET source = 'seed'
 WHERE NOT EXISTS (
   SELECT 1 FROM audit_log a
    WHERE a.entity_type = 'ai_model_rate' AND a.entity_id = r.id
      AND a.actor_type = 'human' AND a.action IN ('create', 'update'));
