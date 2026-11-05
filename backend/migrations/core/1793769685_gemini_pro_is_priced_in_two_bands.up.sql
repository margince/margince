SET LOCAL lock_timeout = '3s';
-- gemini-3.1-pro-preview costs $2/$12 per MTok up to 200k prompt tokens and
-- $4/$18 above. An installation provisioned before rates had a long-context
-- band has no row for it, or one at the flat $2/$12 a seed or the catalogue
-- once wrote, which under-prices every long call by half.
--
-- A provider whose sheet prices anything gets the banded row when it has none.
-- A row still at that exact flat price gains the band. Any other price is
-- somebody's own and is left alone. Both writes are audited as the migration's.
WITH added AS (
  INSERT INTO ai_model_rate (provider, model_id, lane, input_per_mtok_microusd, output_per_mtok_microusd,
                             cache_read_per_mtok_microusd, cache_write_per_mtok_microusd, effective_date, source,
                             long_context_above_tokens, long_context_input_per_mtok_microusd,
                             long_context_output_per_mtok_microusd, long_context_cache_read_per_mtok_microusd,
                             long_context_cache_write_per_mtok_microusd)
  SELECT p.provider, 'gemini-3.1-pro-preview', 'chat', 2000000, 12000000, 200000, 0, CURRENT_DATE, 'seed',
         200000, 4000000, 18000000, 400000, 0
    FROM (VALUES ('gemini'), ('gemini_vertex')) p(provider)
   WHERE EXISTS (SELECT 1 FROM ai_model_rate r WHERE r.provider = p.provider)
     AND NOT EXISTS (SELECT 1 FROM ai_model_rate r
                      WHERE r.provider = p.provider AND r.model_id = 'gemini-3.1-pro-preview')
  ON CONFLICT (provider, model_id, effective_date) DO NOTHING
  RETURNING id, provider, effective_date
), banded AS (
  UPDATE ai_model_rate
     SET long_context_above_tokens = 200000, long_context_input_per_mtok_microusd = 4000000,
         long_context_output_per_mtok_microusd = 18000000, long_context_cache_read_per_mtok_microusd = 400000,
         long_context_cache_write_per_mtok_microusd = 0
   WHERE provider IN ('gemini', 'gemini_vertex') AND model_id = 'gemini-3.1-pro-preview'
     AND source IN ('seed', 'catalogue') AND long_context_above_tokens IS NULL
     AND input_per_mtok_microusd = 2000000 AND output_per_mtok_microusd = 12000000
     AND cache_read_per_mtok_microusd = 200000 AND cache_write_per_mtok_microusd = 0
  RETURNING id, provider, effective_date
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'create', 'ai_model_rate', added.id, NULL,
       jsonb_build_object('provider', added.provider, 'model_id', 'gemini-3.1-pro-preview',
                          'effective_date', added.effective_date, 'long_context_above_tokens', 200000)
  FROM added
UNION ALL
SELECT 'system', 'migration', 'update', 'ai_model_rate', banded.id,
       jsonb_build_object('long_context_above_tokens', NULL),
       jsonb_build_object('provider', banded.provider, 'model_id', 'gemini-3.1-pro-preview',
                          'effective_date', banded.effective_date, 'long_context_above_tokens', 200000)
  FROM banded;
