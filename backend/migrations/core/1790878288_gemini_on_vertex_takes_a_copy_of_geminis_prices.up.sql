SET LOCAL lock_timeout = '3s';
-- Gemini on Vertex AI serves Gemini's models. An installation whose Vertex
-- price sheet is still empty takes a copy of every Gemini row, history and
-- future-dated rows included, so Vertex keeps its prices if the Gemini rows
-- are later removed. One that already prices Vertex is left alone, and a
-- migration never runs twice, so a sheet an admin empties stays empty.
WITH copied AS (
  INSERT INTO ai_model_rate (provider, model_id, lane, input_per_mtok_microusd, output_per_mtok_microusd,
                             cache_read_per_mtok_microusd, cache_write_per_mtok_microusd, effective_date)
  SELECT 'gemini_vertex', g.model_id, g.lane, g.input_per_mtok_microusd, g.output_per_mtok_microusd,
         g.cache_read_per_mtok_microusd, g.cache_write_per_mtok_microusd, g.effective_date
    FROM ai_model_rate g
   WHERE g.provider = 'gemini'
     AND NOT EXISTS (SELECT 1 FROM ai_model_rate v WHERE v.provider = 'gemini_vertex')
  ON CONFLICT (provider, model_id, effective_date) DO NOTHING
  RETURNING id, model_id, lane, effective_date, input_per_mtok_microusd, output_per_mtok_microusd,
            cache_read_per_mtok_microusd, cache_write_per_mtok_microusd
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'create', 'ai_model_rate', copied.id, NULL,
       jsonb_build_object('provider', 'gemini_vertex', 'model_id', copied.model_id, 'lane', copied.lane,
                          'effective_date', copied.effective_date, 'copied_from', 'gemini',
                          'input_per_mtok_microusd', copied.input_per_mtok_microusd,
                          'output_per_mtok_microusd', copied.output_per_mtok_microusd,
                          'cache_read_per_mtok_microusd', copied.cache_read_per_mtok_microusd,
                          'cache_write_per_mtok_microusd', copied.cache_write_per_mtok_microusd)
  FROM copied;
