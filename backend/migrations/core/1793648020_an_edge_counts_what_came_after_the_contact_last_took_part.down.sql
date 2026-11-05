SET LOCAL lock_timeout = '3s';

ALTER TABLE graph_interaction_edge
  DROP COLUMN late_count_90d,
  DROP COLUMN late_out_count_90d;
