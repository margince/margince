-- A frozen base amount is a conversion, and a conversion needs both scales.
--
-- deal.amount_minor_base was GENERATED ALWAYS AS
-- round(amount_minor * fx_rate_to_base), which multiplies a count of MINOR
-- units by a MAJOR-unit rate. That is only correct while both currencies carry
-- two decimals. A closed VND deal (zero decimals) against a EUR base (two) came
-- out a hundred times too small, and the number is STORED, so every reader of
-- the column repeated it: the projects rollup, the organization won-lifetime
-- figure, the weekly comparison and the report engine's own frozen branch.
--
-- The scales cannot be reached from a generated column. A generated expression
-- may read only the row it belongs to, and both scales live elsewhere —
-- currency_minor_digits for the deal's currency, and the installation's base
-- currency setting for the other half. So the column stops being generated and
-- becomes one the freeze writer fills, alongside the rate it froze at.
--
-- fx_rate_to_base keeps its meaning exactly: one MAJOR unit of the deal's
-- currency in MAJOR units of the base. Redefining it as a minor-unit ratio
-- would silently change every row already written and every rate an admin
-- loads from a rates page.
--
-- The arithmetic is the one 1788312487 already established for
-- organization_open_pipeline_rollup, which converts open deals correctly:
--   round(amount_minor * rate * 10^base_digits / 10^deal_digits)
-- one rounding, both scales, ISO's default of two where a code is absent.

SET LOCAL lock_timeout = '5s';

-- Drop the generated column and add a plain one. A generated column cannot be
-- converted in place; the values it held are recomputed below.
ALTER TABLE deal DROP COLUMN amount_minor_base;
ALTER TABLE deal ADD COLUMN amount_minor_base bigint;

COMMENT ON COLUMN deal.amount_minor_base IS
  'The deal amount in the installation base currency, frozen at close. Written by the freeze writer (deals.freezeBaseRate) across both currencies minor-unit scales, never generated: a generated expression cannot read currency_minor_digits or the installation base. NULL while a deal is open — an open deal converts at the latest rate instead, and deal_closed_fx requires a frozen rate only once it closes.';

-- Recompute what the generated column held, correctly this time.
--
-- The base currency is the installation setting, read here rather than assumed
-- EUR: an installation that runs in JPY would otherwise be backfilled against
-- the wrong scale. A row whose rate is missing stays NULL, which is what the
-- readers already treat as "no frozen figure".
UPDATE deal d
   SET amount_minor_base = round(
         d.amount_minor * d.fx_rate_to_base
           * power(10::numeric, coalesce(
               (SELECT bd.digits FROM currency_minor_digits bd
                 WHERE bd.currency = (SELECT value #>> '{}' FROM setting
                                       WHERE key = 'installation.base_currency')), 2))
           / power(10::numeric, coalesce(
               (SELECT dd.digits FROM currency_minor_digits dd
                 WHERE dd.currency = d.currency), 2)))::bigint
 WHERE d.fx_rate_to_base IS NOT NULL
   AND d.amount_minor IS NOT NULL;
