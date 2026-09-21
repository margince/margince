-- Nine index names describe a column no table has.
--
-- The `ws` segment is `workspace` — the tenant column ADR-0091 §8 phase D
-- dropped from every table in this schema (1787320004). The column is gone and
-- the names still point at it.
--
-- An index name is not internal. It appears in EXPLAIN output, in the
-- constraint-violation error that reaches a caller, and in the
-- pg_stat_user_indexes row an operator reads when deciding whether an index
-- earns its keep. A name saying `ws` sends that reader looking for a column
-- that does not exist; `idx_dsh_ws_time` names neither its table nor its column
-- truthfully.
--
-- RENAME rather than drop and recreate: it rewrites no data and takes a brief
-- lock, where a recreate would rebuild every one of these.
--
-- This is not a naming sweep. Five conventions compete in this schema
-- (`idx_*`, bare descriptive, `uq_*`, Postgres-generated `*_key`, `*_idx`) and
-- renaming 650 indexes would be a large diff for no correctness gain. What is
-- here is the names that are actively wrong, plus one abbreviation beside them
-- that costs a reader a lookup for nothing.
--
-- The nine `uq_*_ws_id` duplicates are NOT here. Each was UNIQUE (workspace_id,
-- id) and collapsed to UNIQUE (id), which the primary key already enforces, so
-- they are dropped rather than renamed and belong to that work.
--
-- `idx_import_run_ws` is not here either, and not for that reason: it is
-- created by the CUSTOM namespace, which a core migration may not assume has
-- run. It is renamed one namespace over, in the same change.
SET LOCAL lock_timeout = '3s';

ALTER INDEX idx_activity_ws_time      RENAME TO idx_activity_occurred_live;
ALTER INDEX idx_lead_ws_live          RENAME TO idx_lead_status_live;
ALTER INDEX idx_dsh_ws_time           RENAME TO idx_deal_stage_history_changed;
ALTER INDEX ai_call_ws_time           RENAME TO idx_ai_call_occurred;
ALTER INDEX ai_call_ws_corr           RENAME TO idx_ai_call_correlation;
ALTER INDEX ai_call_ws_run            RENAME TO idx_ai_call_agent_run;
ALTER INDEX uq_channel_connection_ws  RENAME TO uq_channel_connection_provider;

-- `cand` is `candidate`, abbreviated for no gain — the same reason `dsh` above
-- is spelled out.
ALTER INDEX idx_lead_cand_company     RENAME TO idx_lead_candidate_company;
