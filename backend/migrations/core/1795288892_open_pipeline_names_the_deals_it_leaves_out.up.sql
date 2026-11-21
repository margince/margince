-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

SET LOCAL lock_timeout = '3s';

-- company_open_pipeline_rollup counts the open deals its sum leaves out but not
-- why. This reads the same two counts and splits them by cause: the deals with
-- no amount, and the deals with an amount no rate converts. It lists the
-- currencies that have no rate on or before the date, so a reader of a partial
-- total can name what is missing.
--
-- The no-rate count is the rollup's own arithmetic (open minus priced minus no
-- amount), so it can never disagree with the total it qualifies. Only the
-- currency list reads the rate sheet, and it states a currency, not a deal.
CREATE FUNCTION company_open_pipeline_gaps(as_of date)
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
                             AND f.rate_date <= as_of)) AS currencies
       FROM deal d
       LEFT JOIN LATERAL (
         SELECT (value #>> '{}')::text AS code
           FROM setting WHERE key = 'installation.base_currency'
       ) base ON true
      WHERE d.company_id = r.company_id AND d.status = 'open' AND d.archived_at IS NULL
   ) v ON true
$fn$;
GRANT EXECUTE ON FUNCTION company_open_pipeline_gaps(date) TO margince_app;
COMMENT ON FUNCTION company_open_pipeline_gaps(date) IS
  'Why company_open_pipeline_rollup leaves open deals out of its sum: deals with no amount, deals whose currency has no rate on or before the date, and those currencies.';
