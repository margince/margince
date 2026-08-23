-- Back to the column-only sum: the shape the baseline created, byte for byte,
-- so a down-and-up leaves the schema where it started.
SET LOCAL lock_timeout = '5s';

CREATE OR REPLACE VIEW organization_open_pipeline_rollup WITH (security_invoker='true') AS
 SELECT organization_id,
    sum(amount_minor_base) AS open_pipeline_minor_base,
    count(*) AS open_deal_count
   FROM deal d
  WHERE ((status = 'open'::text) AND (organization_id IS NOT NULL) AND (archived_at IS NULL))
  GROUP BY organization_id;

COMMENT ON VIEW organization_open_pipeline_rollup IS NULL;
