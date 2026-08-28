-- Back to the view that raises on an unrepresentable converted amount.
SET LOCAL lock_timeout = '5s';

DROP VIEW IF EXISTS organization_open_pipeline_rollup;

CREATE VIEW organization_open_pipeline_rollup WITH (security_invoker='true') AS
 SELECT d.organization_id,
    sum(
      CASE
        WHEN d.currency = base.code THEN d.amount_minor
        ELSE round(d.amount_minor * live.rate)::bigint
      END
    ) AS open_pipeline_minor_base,
    count(*) AS open_deal_count,
    count(*) FILTER (
      WHERE d.amount_minor IS NOT NULL
        AND (d.currency = base.code OR live.rate IS NOT NULL)
    ) AS priced_deal_count
   FROM deal d
   LEFT JOIN LATERAL (
     SELECT (value #>> '{}')::text AS code
       FROM setting
      WHERE key = 'installation.base_currency'
   ) base ON true
   LEFT JOIN LATERAL (
     SELECT r.rate
       FROM fx_rate r
      WHERE d.currency IS DISTINCT FROM base.code
        AND r.from_currency = d.currency
        AND r.to_currency = base.code
        AND r.rate_date <= CURRENT_DATE
      ORDER BY r.rate_date DESC
      LIMIT 1
   ) live ON true
  WHERE ((d.status = 'open'::text) AND (d.organization_id IS NOT NULL) AND (d.archived_at IS NULL))
  GROUP BY d.organization_id;
