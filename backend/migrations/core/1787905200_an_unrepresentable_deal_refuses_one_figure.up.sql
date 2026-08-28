-- One deal no longer takes the whole company record down.
--
-- The view cast its converted amount with `round(d.amount_minor * live.rate)
-- ::bigint`. Postgres raises `numeric field overflow` on a result that does not
-- fit — so an implausible amount against a large rate failed the STATEMENT,
-- which failed GetOrganization, which made the entire organization unreadable.
-- One bad row, and the record it sits on cannot be opened at all.
--
-- It now contributes NOTHING and stays counted, which is the answer the view
-- already gives a deal whose currency has no usable rate. The page reports a
-- partial figure with priced_deal_count beside it, and that column is what
-- makes "partial" visible rather than inferred.
--
-- The Go path has always done this (org360/pipelinefold.go's wouldWrap), and
-- its comment says why in one line worth repeating: a total that wraps is a
-- plausible-looking wrong number, which is worse than no number. This makes the
-- two agree.
--
-- The conversion moves into a LATERAL so the bound is written ONCE. It was
-- spelled in the sum and would have needed spelling again in the count
-- predicate — two copies of one rule, and a count that disagreed with its own
-- sum would report a figure as complete while it was short.
--
-- DROP then CREATE, not CREATE OR REPLACE: replace cannot change a view's
-- column list, and while these names are unchanged the shape below is rebuilt
-- rather than edited.
SET LOCAL lock_timeout = '5s';

DROP VIEW IF EXISTS organization_open_pipeline_rollup;

CREATE VIEW organization_open_pipeline_rollup WITH (security_invoker='true') AS
 SELECT d.organization_id,
    -- The SUM is bounded too, and separately from its summands. sum(bigint)
    -- answers in numeric, so a set of individually representable deals can add
    -- to a figure no bigint holds — and the reader scans this column into an
    -- int64. Guarding one deal and not the total would have moved the same
    -- failure one level up: the record unreadable because the deals are large
    -- rather than because one of them is.
    --
    -- Out of range answers NULL, which the caller already knows how to report:
    -- the deals were priced, the total cannot be stated, and a figure nobody
    -- can represent is not a figure to publish.
    CASE
      WHEN sum(conv.minor_base)
           BETWEEN -9223372036854775808 AND 9223372036854775807
        THEN sum(conv.minor_base)
      ELSE NULL
    END AS open_pipeline_minor_base,
    count(*) AS open_deal_count,
    -- How many deals actually reached the sum. SUM ignores a null summand
    -- silently, so without this a total covering one of two deals is
    -- indistinguishable from one covering both — a confident figure that is
    -- quietly short, which is worse than the "not computable" it replaces.
    --
    -- It counts the CONVERSION rather than restating when one is possible: a
    -- deal with no rate and a deal whose converted amount does not fit are both
    -- deals the sum did not reach, and one predicate answers for both.
    count(*) FILTER (WHERE conv.minor_base IS NOT NULL) AS priced_deal_count
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
   -- What this deal contributes to the total, or NULL when it contributes
   -- nothing. Three cases, and the third is the one this migration adds.
   LEFT JOIN LATERAL (
     SELECT CASE
       -- Already in the installation's own currency: no rate needed, and none
       -- should be looked for. This is the ordinary deal.
       WHEN d.currency = base.code THEN d.amount_minor
       -- Converted, and only when the result is a number the column can hold.
       -- The comparison runs in numeric, where the product already is, so it
       -- decides the question BEFORE a cast can raise it.
       WHEN live.rate IS NOT NULL
        AND round(d.amount_minor * live.rate)
            BETWEEN -9223372036854775808 AND 9223372036854775807
         THEN round(d.amount_minor * live.rate)::bigint
       -- No usable rate, or a result too large to represent. Nothing is ever
       -- converted at an invented rate of 1 — that would report ¥5,000,000 as
       -- €5,000,000 — and nothing is ever clamped to the biggest number that
       -- fits, which is the same lie with more digits.
       ELSE NULL
     END AS minor_base
   ) conv ON true
  WHERE ((d.status = 'open'::text) AND (d.organization_id IS NOT NULL) AND (d.archived_at IS NULL))
  GROUP BY d.organization_id;

-- DROP took the view's comment with it. Restated rather than left off: it is
-- what a reader querying the schema is told the view means, and the sentence is
-- unchanged except for the case this migration adds.
COMMENT ON VIEW organization_open_pipeline_rollup IS
  'Open pipeline per organization in the installation base currency. Open deals hold no frozen rate — that happens on close — so each foreign-currency deal converts at the latest fx_rate on or before today. A deal with no usable rate, or whose converted amount does not fit a bigint, contributes nothing and is still counted in open_deal_count, so a partial sum is detectable rather than silently short. The total itself answers NULL when it does not fit either.';
