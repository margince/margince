SET LOCAL lock_timeout = '3s';

ALTER TABLE activity RESET (fillfactor);
ALTER TABLE activity_participant RESET (autovacuum_vacuum_scale_factor, autovacuum_analyze_scale_factor);
ALTER TABLE graph_interaction_edge RESET (autovacuum_vacuum_scale_factor, autovacuum_analyze_scale_factor);
ALTER TABLE capture_trace RESET (autovacuum_vacuum_scale_factor, autovacuum_analyze_scale_factor);
ALTER TABLE idempotency_key RESET (
    autovacuum_vacuum_scale_factor, autovacuum_analyze_scale_factor, toast.autovacuum_vacuum_scale_factor);
ALTER TABLE event_outbox RESET (
    autovacuum_vacuum_scale_factor, autovacuum_analyze_scale_factor, toast.autovacuum_vacuum_scale_factor);
