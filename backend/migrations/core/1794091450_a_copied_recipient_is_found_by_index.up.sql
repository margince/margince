SET LOCAL lock_timeout = '3s';

-- The addresses a send from Margince carried, folded to lower case, for the
-- index the next migration builds. It keeps the proof read's own rule: a NULL
-- recipients or cc lists no address.
CREATE FUNCTION comms_outbound_addressed(recipients jsonb, cc jsonb)
RETURNS text[] LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT array_agg(lower(address))
      FROM jsonb_array_elements_text(
          CASE WHEN jsonb_typeof(recipients || cc) = 'array' THEN recipients || cc END) address
$$;

-- A counterparty stored before capture folded it, folded, and NULL for every
-- other row. The statistics tell the planner that almost every row is NULL. A
-- comparison of two columns has no statistics, and its default estimate made
-- the proof read start a parallel worker for an empty index.
CREATE FUNCTION activity_unfolded_counterparty(address text)
RETURNS text LANGUAGE sql IMMUTABLE PARALLEL SAFE AS $$
    SELECT CASE WHEN address <> lower(address) THEN lower(address) END
$$;
CREATE STATISTICS activity_unfolded_counterparty_stats
    ON (activity_unfolded_counterparty(counterparty_email)) FROM activity;
ANALYZE activity;
