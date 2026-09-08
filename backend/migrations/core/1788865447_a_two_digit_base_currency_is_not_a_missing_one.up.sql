-- The frozen-base backfill emptied the column it was written to correct.
--
-- 1788583500 dropped the generated deal.amount_minor_base, added a plain
-- column and recomputed every closed row. Its deal-side scale read
--
--   coalesce((SELECT dd.digits FROM currency_minor_digits dd WHERE ...), 2)
--
-- but its BASE-side scale put the coalesce INSIDE the subquery:
--
--   (SELECT coalesce(bd.digits, 2) FROM currency_minor_digits bd WHERE ...)
--
-- A coalesce inside a scalar subquery never sees a missing row. It defaults a
-- NULL digits COLUMN, which cannot happen — the column is NOT NULL — while an
-- absent row still yields NULL and takes the whole expression with it.
--
-- currency_minor_digits holds only the currencies that DEVIATE from ISO's two
-- decimals, so EUR, USD and GBP are absent by design. Every installation whose
-- base is an ordinary two-decimal currency therefore backfilled its entire
-- closed book to NULL: not the foreign-currency rows alone, but every closed
-- deal including base-currency ones converting at a rate of 1.
--
-- The readers treat NULL as "no frozen figure", so the loss is silent. Closed
-- revenue, the projects rollup, the organization won-lifetime figure and the
-- weekly comparison each drop the row rather than reporting a wrong number.
--
-- The runtime SQL (compose.BaseValueSQL, briefs.briefBaseValueSQL) always had
-- the coalesce in the right place, which is why open-pipeline figures stayed
-- correct while the frozen column emptied underneath them.
--
-- This recomputes the same rows with the same arithmetic and the coalesce
-- outside, where it answers the question actually being asked. A row the
-- arithmetic cannot answer for stays NULL exactly as before: no rate, no
-- amount, no base-currency setting, or a product too large for bigint.
--
-- Rows the freeze writer has already filled since 1788583500 ran are recomputed
-- to the identical value: the writer uses this same arithmetic, so a correct
-- row survives the pass unchanged.

SET LOCAL lock_timeout = '5s';

UPDATE deal d
   SET amount_minor_base = CASE
         WHEN round(d.amount_minor * d.fx_rate_to_base
                * power(10::numeric, coalesce(
                    (SELECT bd.digits FROM currency_minor_digits bd
                      WHERE bd.currency = base.code), 2))
                / power(10::numeric, coalesce(
                    (SELECT dd.digits FROM currency_minor_digits dd
                      WHERE dd.currency = d.currency), 2)))
              BETWEEN -9223372036854775808 AND 9223372036854775807
         THEN round(d.amount_minor * d.fx_rate_to_base
                * power(10::numeric, coalesce(
                    (SELECT bd.digits FROM currency_minor_digits bd
                      WHERE bd.currency = base.code), 2))
                / power(10::numeric, coalesce(
                    (SELECT dd.digits FROM currency_minor_digits dd
                      WHERE dd.currency = d.currency), 2)))::bigint
         ELSE NULL
       END
  FROM (SELECT value #>> '{}' AS code FROM setting
         WHERE key = 'installation.base_currency') AS base
 WHERE d.fx_rate_to_base IS NOT NULL
   AND d.amount_minor IS NOT NULL;
