-- Back to two types and ten copies of the rule, each column as this migration
-- found it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE assurance_exception ALTER COLUMN currency TYPE text USING currency::text;
ALTER TABLE commission_entry ALTER COLUMN currency TYPE text USING currency::text;
ALTER TABLE contract ALTER COLUMN currency TYPE character(3) USING currency::character(3);
ALTER TABLE currency_minor_digits ALTER COLUMN currency TYPE character(3) USING currency::character(3);
ALTER TABLE custom_field ALTER COLUMN currency TYPE character(3) USING currency::character(3);
ALTER TABLE deal ALTER COLUMN currency TYPE character(3) USING currency::character(3);
ALTER TABLE deal_stage_history ALTER COLUMN base_currency_at_change TYPE text USING base_currency_at_change::text;
ALTER TABLE deal_stage_history ALTER COLUMN currency_at_change TYPE character(3) USING currency_at_change::character(3);
ALTER TABLE deal_suggestion ALTER COLUMN currency TYPE text USING currency::text;
ALTER TABLE finance_invoice ALTER COLUMN currency TYPE character(3) USING currency::character(3);
ALTER TABLE finance_payment ALTER COLUMN currency TYPE character(3) USING currency::character(3);
ALTER TABLE forecast_call ALTER COLUMN currency TYPE text USING currency::text;
ALTER TABLE forecast_contribution ALTER COLUMN currency TYPE text USING currency::text;
ALTER TABLE forecast_snapshot ALTER COLUMN base_currency TYPE text USING base_currency::text;
ALTER TABLE fx_rate ALTER COLUMN from_currency TYPE character(3) USING from_currency::character(3);
ALTER TABLE fx_rate ALTER COLUMN to_currency TYPE character(3) USING to_currency::character(3);
ALTER TABLE legal_review ALTER COLUMN value_currency TYPE character(3) USING value_currency::character(3);
ALTER TABLE offer ALTER COLUMN currency TYPE character(3) USING currency::character(3);
ALTER TABLE product ALTER COLUMN currency TYPE character(3) USING currency::character(3);
ALTER TABLE team_weekly_review ALTER COLUMN base_currency TYPE text USING base_currency::text;
ALTER TABLE team_weekly_review_coverage ALTER COLUMN base_currency TYPE text USING base_currency::text;
ALTER TABLE team_weekly_review_outlook ALTER COLUMN base_currency TYPE text USING base_currency::text;
ALTER TABLE weekly_review ALTER COLUMN base_currency TYPE text USING base_currency::text;
ALTER TABLE weekly_review_deal ALTER COLUMN currency_at_close TYPE text USING currency_at_close::text;
ALTER TABLE weekly_review_outlook ALTER COLUMN base_currency TYPE text USING base_currency::text;

ALTER TABLE commission_entry ADD CONSTRAINT commission_entry_currency_check CHECK (currency IS NULL OR currency ~ '^[A-Z]{3}$') NOT VALID;
ALTER TABLE custom_field ADD CONSTRAINT custom_field_currency_check CHECK (currency IS NULL OR currency ~ '^[A-Z]{3}$') NOT VALID;
ALTER TABLE deal ADD CONSTRAINT deal_currency_check CHECK (currency IS NULL OR currency ~ '^[A-Z]{3}$') NOT VALID;
ALTER TABLE finance_invoice ADD CONSTRAINT finance_invoice_currency_check CHECK (currency IS NULL OR currency ~ '^[A-Z]{3}$') NOT VALID;
ALTER TABLE finance_payment ADD CONSTRAINT finance_payment_currency_check CHECK (currency IS NULL OR currency ~ '^[A-Z]{3}$') NOT VALID;
ALTER TABLE offer ADD CONSTRAINT offer_currency_check CHECK (currency IS NULL OR currency ~ '^[A-Z]{3}$') NOT VALID;
ALTER TABLE product ADD CONSTRAINT product_currency_check CHECK (currency IS NULL OR currency ~ '^[A-Z]{3}$') NOT VALID;
ALTER TABLE team_weekly_review ADD CONSTRAINT team_weekly_review_currency_check CHECK (base_currency IS NULL OR base_currency ~ '^[A-Z]{3}$') NOT VALID;
ALTER TABLE weekly_review ADD CONSTRAINT weekly_review_currency_check CHECK (base_currency IS NULL OR base_currency ~ '^[A-Z]{3}$') NOT VALID;

DROP DOMAIN currency_code;
