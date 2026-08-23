-- Back to the column-only sum: the shape the baseline created, byte for byte,
-- so a down-and-up leaves the schema where it started.
--
-- DROP then CREATE, not CREATE OR REPLACE: the up migration ADDED a column, and
-- replace cannot remove one — Postgres refuses a replacement that drops an
-- output column, so a plain replace here would fail rather than roll back.
SET LOCAL lock_timeout = '5s';

DROP VIEW IF EXISTS organization_open_pipeline_rollup;

CREATE VIEW organization_open_pipeline_rollup WITH (security_invoker='true') AS
 SELECT organization_id,
    sum(amount_minor_base) AS open_pipeline_minor_base,
    count(*) AS open_deal_count
   FROM deal d
  WHERE ((status = 'open'::text) AND (organization_id IS NOT NULL) AND (archived_at IS NULL))
  GROUP BY organization_id;
