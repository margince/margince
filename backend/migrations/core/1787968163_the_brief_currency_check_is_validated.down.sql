-- Put the constraint back the way this migration found it: present, unvalidated.
--
-- `SELECT 1` would leave it VALIDATED while the migration reads unapplied, so a
-- rollback to test the pair would find the state the forward pass produced and
-- prove nothing. Postgres has no ALTER that un-validates, so the constraint is
-- dropped and re-added NOT VALID — which takes ACCESS EXCLUSIVE without a scan.
--
-- The rows still satisfy it; what is being restored is the CATALOG's record of
-- whether anybody has checked.
SET LOCAL lock_timeout = '3s';

ALTER TABLE brief_run
    DROP CONSTRAINT brief_run_revenue_norm_currency_check,
    ADD CONSTRAINT brief_run_revenue_norm_currency_check
        CHECK (revenue_norm_currency = '' OR revenue_norm_currency ~ '^[A-Z]{3}$') NOT VALID;
