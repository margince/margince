SET LOCAL lock_timeout = '3s';

ALTER TABLE relationship DROP CONSTRAINT rel_dates;
ALTER TABLE relationship ADD CONSTRAINT rel_dates CHECK (
 ended_at IS NULL OR started_at IS NULL OR
 CASE WHEN ended_precision = 'month' THEN (date_trunc('month', ended_at) + interval '1 month')::date > started_at
 ELSE ended_at >= started_at END
);
