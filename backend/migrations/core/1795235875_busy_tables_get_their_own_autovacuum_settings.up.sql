-- Small tables that are rewritten all day pass the default threshold, 20% of
-- the table, only after their indexes have grown to several times the data.
-- SET takes SHARE UPDATE EXCLUSIVE, which readers and writers pass.
SET LOCAL lock_timeout = '3s';

ALTER TABLE event_outbox SET (
    autovacuum_vacuum_scale_factor = 0.02, autovacuum_analyze_scale_factor = 0.02,
    toast.autovacuum_vacuum_scale_factor = 0.02);
ALTER TABLE idempotency_key SET (
    autovacuum_vacuum_scale_factor = 0.02, autovacuum_analyze_scale_factor = 0.02,
    toast.autovacuum_vacuum_scale_factor = 0.02);
ALTER TABLE capture_trace SET (
    autovacuum_vacuum_scale_factor = 0.02, autovacuum_analyze_scale_factor = 0.02);
ALTER TABLE graph_interaction_edge SET (
    autovacuum_vacuum_scale_factor = 0.02, autovacuum_analyze_scale_factor = 0.02);
ALTER TABLE activity_participant SET (
    autovacuum_vacuum_scale_factor = 0.02, autovacuum_analyze_scale_factor = 0.02);

-- Room on each page for the next version of a row, so a verdict stamp on a
-- mail stays on its page. It applies to pages written from now on.
ALTER TABLE activity SET (fillfactor = 85);
