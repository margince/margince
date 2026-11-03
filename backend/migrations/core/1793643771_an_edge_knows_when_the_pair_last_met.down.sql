SET LOCAL lock_timeout = '3s';

ALTER TABLE graph_interaction_edge DROP COLUMN last_met_at;
