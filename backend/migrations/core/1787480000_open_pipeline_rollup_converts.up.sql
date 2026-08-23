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
-- counted. The view reports priced_deal_count beside open_deal_count so a
-- caller can tell a complete figure from a partial one rather than inferring
-- it: SUM ignores a null summand without saying so. Nothing is ever converted
-- at an invented rate of 1 — that would report ¥5,000,000 as €5,000,000.
--
-- security_invoker stays true, and what it means here is narrower than it was.
-- The view now reads setting and fx_rate as well as deal, so it no longer reads
-- "no more than a direct SELECT on deal could": security_invoker checks the
-- database role, which every application request shares, not the authenticated
-- principal. What it discloses is a CONVERTED total — from which a reader who
-- already knows one foreign deal's own amount could infer the rate applied to
-- it. That is the same derivation the company page's open-pipeline figure
-- already publishes to the same readers, so this adds no channel; it is written
-- down because the module reading this view says the organization_id is the
-- scope, and that sentence is now about deal alone.
-- DROP then CREATE, not CREATE OR REPLACE: this ADDS an output column, and
-- replace cannot change a view's column list.
SET LOCAL lock_timeout = '5s';

DROP VIEW IF EXISTS organization_open_pipeline_rollup;

CREATE VIEW organization_open_pipeline_rollup WITH (security_invoker='true') AS
 SELECT d.organization_id,
    sum(
      CASE
        -- Already in the installation's own currency: no rate needed, and none
        -- should be looked for. This is the ordinary deal.
        WHEN d.currency = base.code THEN d.amount_minor
        ELSE round(d.amount_minor * live.rate)::bigint
      END
    ) AS open_pipeline_minor_base,
    count(*) AS open_deal_count,
    -- How many deals actually reached the sum. SUM ignores a null summand
    -- silently, so without this a total covering one of two deals is
    -- indistinguishable from one covering both — a confident figure that is
    -- quietly short, which is worse than the "not computable" it replaces.
    count(*) FILTER (
      WHERE d.amount_minor IS NOT NULL
        AND (d.currency = base.code OR live.rate IS NOT NULL)
    ) AS priced_deal_count
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
