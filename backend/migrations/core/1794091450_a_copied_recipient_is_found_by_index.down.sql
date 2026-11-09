SET LOCAL lock_timeout = '3s';

DROP STATISTICS IF EXISTS activity_unfolded_counterparty_stats;
DROP FUNCTION IF EXISTS activity_unfolded_counterparty(text);
DROP FUNCTION IF EXISTS comms_outbound_addressed(jsonb, jsonb);
