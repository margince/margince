SET LOCAL lock_timeout = '3s';

ALTER INDEX idx_activity_occurred_live       RENAME TO idx_activity_ws_time;
ALTER INDEX idx_lead_status_live             RENAME TO idx_lead_ws_live;
ALTER INDEX idx_deal_stage_history_changed   RENAME TO idx_dsh_ws_time;
ALTER INDEX idx_ai_call_occurred             RENAME TO ai_call_ws_time;
ALTER INDEX idx_ai_call_correlation          RENAME TO ai_call_ws_corr;
ALTER INDEX idx_ai_call_agent_run            RENAME TO ai_call_ws_run;
ALTER INDEX uq_channel_connection_provider   RENAME TO uq_channel_connection_ws;
ALTER INDEX idx_lead_candidate_company       RENAME TO idx_lead_cand_company;
