-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

-- A correction supersedes the day's rate instead of overwriting it, so a figure
-- computed at the old rate can be reproduced. A NULL superseded_at marks the
-- current version; existing rows are all current.
ALTER TABLE fx_rate ADD COLUMN IF NOT EXISTS superseded_at timestamptz;

CREATE UNIQUE INDEX IF NOT EXISTS fx_rate_current_pair_day
    ON fx_rate (from_currency, to_currency, rate_date)
    WHERE superseded_at IS NULL;

ALTER TABLE fx_rate DROP CONSTRAINT IF EXISTS fx_rate_pair_day;

-- The SQL readers pick the current version of a day's rate, as every Go reader
-- does, so a same-day correction cannot leave two answers for one company.
CREATE OR REPLACE FUNCTION company_open_pipeline_rollup(as_of date)
RETURNS TABLE (
  company_id uuid,
  open_pipeline_minor_base bigint,
  open_deal_count bigint,
  priced_deal_count bigint
)
LANGUAGE sql
STABLE
SECURITY INVOKER
AS $fn$
 SELECT d.company_id,
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
   -- all for the company, which reads as "no open deals" — the one answer
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
        AND r.rate_date <= as_of
        AND r.superseded_at IS NULL
      ORDER BY r.rate_date DESC
      LIMIT 1
   ) live ON true
   -- Both currencies' minor-unit scales, each absent for an ordinary two-digit
   -- code and coalesced to ISO's default below. LEFT so a code the table does
   -- not name still converts, at two digits, rather than dropping the deal out
   -- of the sum — an unnamed exception renders wrong for that code, where a
   -- dropped deal silently shortens a total for every code.
   LEFT JOIN currency_minor_digits deal_digits ON deal_digits.currency = d.currency
   LEFT JOIN currency_minor_digits base_digits ON base_digits.currency = base.code
   -- What this deal contributes to the total, or NULL when it contributes
   -- nothing. Three cases, and the bound is written once here so the count
   -- predicate below reads the same answer the sum does.
   LEFT JOIN LATERAL (
     SELECT CASE
       -- Already in the installation's own currency: no rate needed, no scale
       -- to cross, and none should be looked for. This is the ordinary deal.
       WHEN d.currency = base.code THEN d.amount_minor
       -- Converted, and only when the result is a number the column can hold.
       -- The comparison runs in numeric, where the product already is, so it
       -- decides the question BEFORE a cast can raise it.
       --
       -- amount × rate × 10^digits(base) ÷ 10^digits(deal), as ONE expression
       -- so the single round() is the only rounding. numeric is exact, so the
       -- intermediate carries no error to accumulate.
       WHEN live.rate IS NOT NULL
        AND round(d.amount_minor * live.rate
                    * power(10::numeric, coalesce(base_digits.digits, 2))
                    / power(10::numeric, coalesce(deal_digits.digits, 2)))
            BETWEEN -9223372036854775808 AND 9223372036854775807
         THEN round(d.amount_minor * live.rate
                      * power(10::numeric, coalesce(base_digits.digits, 2))
                      / power(10::numeric, coalesce(deal_digits.digits, 2)))::bigint
       -- No usable rate, or a result too large to represent. Nothing is ever
       -- converted at an invented rate of 1 — that would report ¥5,000,000 as
       -- €5,000,000 — and nothing is ever clamped to the biggest number that
       -- fits, which is the same lie with more digits.
       ELSE NULL
     END AS minor_base
   ) conv ON true
  WHERE ((d.status = 'open'::text) AND (d.company_id IS NOT NULL) AND (d.archived_at IS NULL))
  GROUP BY d.company_id;
$fn$;

CREATE OR REPLACE FUNCTION company_open_pipeline_gaps(as_of date)
RETURNS TABLE (
  company_id uuid,
  no_value_deal_count bigint,
  no_rate_deal_count bigint,
  no_rate_currencies text
)
LANGUAGE sql
STABLE
SECURITY INVOKER
AS $fn$
 SELECT r.company_id,
        coalesce(v.no_value, 0),
        r.open_deal_count - r.priced_deal_count - coalesce(v.no_value, 0),
        v.currencies
   FROM company_open_pipeline_rollup(as_of) r
   LEFT JOIN LATERAL (
     SELECT count(*) FILTER (WHERE d.amount_minor IS NULL) AS no_value,
            string_agg(DISTINCT d.currency, ',' ORDER BY d.currency)
              FILTER (WHERE d.amount_minor IS NOT NULL
                        AND d.currency IS DISTINCT FROM base.code
                        AND NOT EXISTS (
                          SELECT 1 FROM fx_rate f
                           WHERE f.from_currency = d.currency
                             AND f.to_currency = base.code
                             AND f.rate_date <= as_of
                             AND f.superseded_at IS NULL)) AS currencies
       FROM deal d
       LEFT JOIN LATERAL (
         SELECT (value #>> '{}')::text AS code
           FROM setting WHERE key = 'installation.base_currency'
       ) base ON true
      WHERE d.company_id = r.company_id AND d.status = 'open' AND d.archived_at IS NULL
   ) v ON true
$fn$;
