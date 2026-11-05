-- A currency code has one type and one rule.
--
-- Twenty-five columns held a currency code in two types. Ten carried their own
-- copy of the same regex and fifteen carried nothing, so a column with no rule
-- accepted 'ab1' or three spaces, and a text one accepted any length.
--
-- The domain replaces the copies. A column added later cannot get it wrong,
-- because the rule travels with the type.
--
-- brief_run.revenue_norm_currency does not join the domain. It is NOT NULL
-- with a default of '', so the empty string is how it says a brief has no
-- normalised currency, and its own CHECK admits that. Padded to char(3) the
-- sentinel becomes three spaces, which the domain refuses. It keeps its own
-- rule, and the gate waives it with that reason.
SET LOCAL lock_timeout = '3s';

CREATE DOMAIN currency_code AS char(3)
  CONSTRAINT currency_code_shape CHECK (VALUE ~ '^[A-Z]{3}$');

ALTER TABLE assurance_exception ALTER COLUMN currency TYPE currency_code USING currency::char(3);
ALTER TABLE commission_entry ALTER COLUMN currency TYPE currency_code USING currency::char(3);
ALTER TABLE contract ALTER COLUMN currency TYPE currency_code USING currency::char(3);
ALTER TABLE currency_minor_digits ALTER COLUMN currency TYPE currency_code USING currency::char(3);
ALTER TABLE custom_field ALTER COLUMN currency TYPE currency_code USING currency::char(3);
ALTER TABLE deal ALTER COLUMN currency TYPE currency_code USING currency::char(3);
ALTER TABLE deal_stage_history ALTER COLUMN base_currency_at_change TYPE currency_code USING base_currency_at_change::char(3);
ALTER TABLE deal_stage_history ALTER COLUMN currency_at_change TYPE currency_code USING currency_at_change::char(3);
ALTER TABLE deal_suggestion ALTER COLUMN currency TYPE currency_code USING currency::char(3);
ALTER TABLE finance_invoice ALTER COLUMN currency TYPE currency_code USING currency::char(3);
ALTER TABLE finance_payment ALTER COLUMN currency TYPE currency_code USING currency::char(3);
ALTER TABLE forecast_call ALTER COLUMN currency TYPE currency_code USING currency::char(3);
ALTER TABLE forecast_contribution ALTER COLUMN currency TYPE currency_code USING currency::char(3);
ALTER TABLE forecast_snapshot ALTER COLUMN base_currency TYPE currency_code USING base_currency::char(3);
ALTER TABLE fx_rate ALTER COLUMN from_currency TYPE currency_code USING from_currency::char(3);
ALTER TABLE fx_rate ALTER COLUMN to_currency TYPE currency_code USING to_currency::char(3);
ALTER TABLE legal_review ALTER COLUMN value_currency TYPE currency_code USING value_currency::char(3);
ALTER TABLE offer ALTER COLUMN currency TYPE currency_code USING currency::char(3);
ALTER TABLE product ALTER COLUMN currency TYPE currency_code USING currency::char(3);
ALTER TABLE team_weekly_review ALTER COLUMN base_currency TYPE currency_code USING base_currency::char(3);
ALTER TABLE team_weekly_review_coverage ALTER COLUMN base_currency TYPE currency_code USING base_currency::char(3);
ALTER TABLE team_weekly_review_outlook ALTER COLUMN base_currency TYPE currency_code USING base_currency::char(3);
ALTER TABLE weekly_review ALTER COLUMN base_currency TYPE currency_code USING base_currency::char(3);
ALTER TABLE weekly_review_deal ALTER COLUMN currency_at_close TYPE currency_code USING currency_at_close::char(3);
ALTER TABLE weekly_review_outlook ALTER COLUMN base_currency TYPE currency_code USING base_currency::char(3);

-- The per-table copies, now stated once by the domain.
ALTER TABLE commission_entry DROP CONSTRAINT commission_entry_currency_check;
ALTER TABLE custom_field DROP CONSTRAINT custom_field_currency_check;
ALTER TABLE deal DROP CONSTRAINT deal_currency_check;
ALTER TABLE finance_invoice DROP CONSTRAINT finance_invoice_currency_check;
ALTER TABLE finance_payment DROP CONSTRAINT finance_payment_currency_check;
ALTER TABLE offer DROP CONSTRAINT offer_currency_check;
ALTER TABLE product DROP CONSTRAINT product_currency_check;
ALTER TABLE team_weekly_review DROP CONSTRAINT team_weekly_review_currency_check;
ALTER TABLE weekly_review DROP CONSTRAINT weekly_review_currency_check;
