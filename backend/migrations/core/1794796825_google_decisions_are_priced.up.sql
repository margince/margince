SET LOCAL lock_timeout = '3s';
-- gemini-decision-1 costs $0.10 per MTok of input, and its output is free. An
-- installation provisioned before the gemini_decision provider existed has no
-- row for it, so its calls would read as unpriced.
--
-- An installation whose sheet prices anything gets the seeded row when it has
-- none for the model. One it already has, from a seed or an admin, stays. The
-- write is audited as the migration's.
WITH added AS (
  INSERT INTO ai_model_rate (provider, model_id, lane, input_per_mtok_microusd, output_per_mtok_microusd,
                             cache_read_per_mtok_microusd, cache_write_per_mtok_microusd, effective_date, source)
  SELECT 'gemini_decision', 'gemini-decision-1', 'decisions', 100000, 0, 0, 0, CURRENT_DATE, 'seed'
   WHERE EXISTS (SELECT 1 FROM ai_model_rate)
     AND NOT EXISTS (SELECT 1 FROM ai_model_rate r
                      WHERE r.provider = 'gemini_decision' AND r.model_id = 'gemini-decision-1')
  ON CONFLICT (provider, model_id, effective_date) DO NOTHING
  RETURNING id, effective_date
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'create', 'ai_model_rate', added.id, NULL,
       jsonb_build_object('provider', 'gemini_decision', 'model_id', 'gemini-decision-1', 'lane', 'decisions',
                          'effective_date', added.effective_date, 'input_per_mtok_microusd', 100000,
                          'output_per_mtok_microusd', 0)
  FROM added;
