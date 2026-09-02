-- Two surfaces published a field named open_pipeline_minor_base for the same
-- organization and disagreed about it by a hundredfold.
--
-- A stored fx_rate says what one MAJOR unit is worth: the rate ingestion reads
-- "1 <from> = <rate> <to>" off a rates page. Both amounts either side of it
-- count MINOR units. Multiplying one by the other is correct only while the two
-- currencies carry the same number of minor digits, which every pair in the
-- demo data does and JPY against EUR does not: JPY has no minor unit and EUR
-- has two, so this view read ¥5,000,000 (€30,000) as 30,000 minor units — €300.
--
-- The Go engine (deals.ConvertToBase) now scales both sides. This view is the
-- second spelling of that same rule, read by the company RECORD while org360's
-- card reads the Go path: one account, two figures, both labelled base
-- currency. The previous migration's own comment warned against "reporting
-- ¥5,000,000 as €5,000,000" while the arithmetic below it made the same class
-- of error one scale over.
--
-- SQL cannot reach the digit table in Go, so the table comes to SQL. It is a
-- MIRROR of internal/shared/kernel/values.currencyMinorDigits, held in both
-- directions against the LIVE table by
-- backend/internal/compose/integration/minorunits_integration_test.go — the same
-- arrangement frontend/src/format/minorunits.ts already has, and for the same
-- reason: a second answer nobody compares is a wrong answer waiting for its
-- first zero-decimal currency.
--
-- Only the EXCEPTIONS are stored. ISO 4217's default is two digits, so a code
-- absent from this table carries two, which is what the coalesce below spells
-- and what MinorUnitDigits answers in Go.
--
-- Everything else about the view is 1787905200's, unchanged: the per-deal
-- bound, the bound on the sum itself, the three-case conv lateral. Only the
-- conversion inside that lateral gains the two scales. DROP then CREATE for
-- the reason that migration gives — the shape is rebuilt rather than edited.

SET LOCAL lock_timeout = '5s';

CREATE TABLE currency_minor_digits (
    currency char(3) PRIMARY KEY,
    digits smallint NOT NULL CHECK (digits >= 0 AND digits <= 4)
);

COMMENT ON TABLE currency_minor_digits IS
  'How many minor units a currency has, for the codes where that is not two. A MIRROR of internal/shared/kernel/values.currencyMinorDigits, not a second source: the minor-unit parity test fails in both directions when either side moves. A code absent here carries ISO 4217''s default of two.';

INSERT INTO currency_minor_digits (currency, digits) VALUES
  ('BHD', 3),
  ('BIF', 0),
  ('CLF', 4),
  ('CLP', 0),
  ('DJF', 0),
  ('GNF', 0),
  ('IQD', 3),
  ('ISK', 0),
  ('JOD', 3),
  ('JPY', 0),
  ('KMF', 0),
  ('KRW', 0),
  ('KWD', 3),
  ('LYD', 3),
  ('OMR', 3),
  ('PYG', 0),
  ('RWF', 0),
  ('TND', 3),
  ('UGX', 0),
  ('UYI', 0),
  ('UYW', 4),
  ('VND', 0),
  ('VUV', 0),
  ('XAF', 0),
  ('XAG', 0),
  ('XAU', 0),
  ('XDR', 0),
  ('XOF', 0),
  ('XPD', 0),
  ('XPF', 0),
  ('XPT', 0),
  ('XTS', 0),
  ('XXX', 0);

-- READ ONLY for the application, against the schema's default privileges
-- (0001 grants the app INSERT/UPDATE/DELETE on every new table in public).
-- This table is a mirror of a Go constant, and a row written at runtime is a
-- silent parity break: the digit table would then disagree with the Go one,
-- the minor-unit parity test compares the SHIPPED rows and would still pass,
-- and one currency would convert two ways. Only a migration may change it.
REVOKE INSERT, UPDATE, DELETE ON TABLE currency_minor_digits FROM margince_app;
GRANT SELECT ON TABLE currency_minor_digits TO margince_app;

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
  WHERE ((d.status = 'open'::text) AND (d.organization_id IS NOT NULL) AND (d.archived_at IS NULL))
  GROUP BY d.organization_id;

-- DROP took the view's comment with it. Restated rather than left off: it is
-- what a reader querying the schema is told the view means, and the sentence is
-- unchanged except for the scales this migration adds.
COMMENT ON VIEW organization_open_pipeline_rollup IS
  'Open pipeline per organization in the installation base currency. Open deals hold no frozen rate — that happens on close — so each foreign-currency deal converts at the latest fx_rate on or before today, across both currencies'' minor-unit scales (currency_minor_digits). A deal with no usable rate, or whose converted amount does not fit a bigint, contributes nothing and is still counted in open_deal_count, so a partial sum is detectable rather than silently short. The total itself answers NULL when it does not fit either.';
