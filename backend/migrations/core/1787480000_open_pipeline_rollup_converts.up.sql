-- The open-pipeline formula field reported "awaiting_fx" for almost every
-- organization, and the reason was structural rather than a missing rate.
--
-- The view summed deal.amount_minor_base. That generated column is null on
-- every OPEN deal by design — a deal freezes its conversion rate on close
-- (deal_closed_fx) — so the aggregate was null whenever an account's open deals
-- were priced at all, and the field floored to "not computable yet" for a
-- pipeline that was perfectly computable. Only a deal that had been closed and
-- reopened ever contributed.
--
-- The view now converts, at the latest rate stored on or before today, the same
-- rule the company page's own read applies. It resolves the base currency
-- itself from the installation settings row, because a view takes no
-- parameters; a settings table with one row per key makes that a scalar
-- subquery rather than a join.
--
-- A deal whose currency has no usable rate contributes NOTHING and is still
-- counted, so a caller comparing the sum against open_deal_count can tell a
-- complete figure from a partial one. Nothing is ever converted at an invented
-- rate of 1: that would report ¥5,000,000 as €5,000,000.
--
-- security_invoker stays true. The view runs with the CALLING role's
-- privileges, so it reads no more than a direct SELECT on deal could, which is
-- what lets the reading module treat the organization_id as the scope.
SET LOCAL lock_timeout = '5s';

CREATE OR REPLACE VIEW organization_open_pipeline_rollup WITH (security_invoker='true') AS
 SELECT d.organization_id,
    sum(
      CASE
        -- Already in the installation's own currency: no rate needed, and none
        -- should be looked for. This is the ordinary deal.
        WHEN d.currency = base.code THEN d.amount_minor
        ELSE round(d.amount_minor * live.rate)::bigint
      END
    ) AS open_pipeline_minor_base,
    count(*) AS open_deal_count
   FROM deal d
   -- LEFT, not CROSS: an installation whose base-currency row is somehow absent
   -- must still report its open_deal_count. A cross join would return no row at
   -- all for the organization, which reads as "no open deals" — the one answer
   -- that is definitely wrong. With no base currency nothing converts, every
   -- deal contributes null, and the sum is null: not computable, honestly.
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

COMMENT ON VIEW organization_open_pipeline_rollup IS
  'Open pipeline per organization in the installation base currency. Open deals hold no frozen rate — that happens on close — so each foreign-currency deal converts at the latest fx_rate on or before today. A deal with no usable rate contributes nothing and is still counted in open_deal_count, so a partial sum is detectable rather than silently short.';
