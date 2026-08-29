SET LOCAL lock_timeout = '3s';

ALTER TABLE brief_run
    DROP CONSTRAINT brief_run_revenue_norm_currency_check,
    DROP COLUMN revenue_norm_currency;
