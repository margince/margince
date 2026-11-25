SET LOCAL lock_timeout = '3s';
-- claude-sonnet-5-5 costs $2 / $10 per MTok, and claude-haiku-5-5 costs $0.10 / $0.50
-- for a prompt up to 100k tokens and $0.50 / $2.50 above. An installation
-- provisioned before the two models were priced has no row for either, so
-- their calls read as unpriced.
--
-- An installation whose sheet prices anything gets each seeded row when no row
-- for that model is in force today. One in force, from a seed or an admin,
-- stays. The write is audited as the migration's.
WITH added AS (
  INSERT INTO ai_model_rate (provider, model_id, lane, input_per_mtok_microusd, output_per_mtok_microusd,
                             cache_read_per_mtok_microusd, cache_write_per_mtok_microusd, effective_date, source,
                             long_context_above_tokens, long_context_input_per_mtok_microusd,
                             long_context_output_per_mtok_microusd, long_context_cache_read_per_mtok_microusd,
                             long_context_cache_write_per_mtok_microusd)
  SELECT 'anthropic', m.model_id, 'chat', m.input, m.output, m.cache_read, m.cache_write, CURRENT_DATE, 'seed',
         m.above, m.long_input, m.long_output, m.long_cache_read, m.long_cache_write
    FROM (VALUES
      ('claude-sonnet-5-5', 2000000::bigint, 10000000::bigint, 100000::bigint, 2500000::bigint,
       NULL::int, NULL::bigint, NULL::bigint, NULL::bigint, NULL::bigint),
      ('claude-haiku-5-5', 100000::bigint, 500000::bigint, 10000::bigint, 125000::bigint,
       100000, 500000::bigint, 2500000::bigint, 50000::bigint, 625000::bigint)
    ) m(model_id, input, output, cache_read, cache_write, above, long_input, long_output, long_cache_read, long_cache_write)
   WHERE EXISTS (SELECT 1 FROM ai_model_rate)
     AND NOT EXISTS (SELECT 1 FROM ai_model_rate r
                      WHERE r.provider = 'anthropic' AND r.model_id = m.model_id
                        AND r.effective_date <= CURRENT_DATE)
  ON CONFLICT (provider, model_id, effective_date) DO NOTHING
  RETURNING id, model_id, effective_date
)
INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id, before, after)
SELECT 'system', 'migration', 'create', 'ai_model_rate', added.id, NULL,
       jsonb_build_object('provider', 'anthropic', 'model_id', added.model_id, 'lane', 'chat',
                          'effective_date', added.effective_date)
  FROM added;
