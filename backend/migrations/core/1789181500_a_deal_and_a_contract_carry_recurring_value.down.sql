-- Both tables are live and written constantly, so the ALTERs below queue behind
-- any open transaction touching them. Bounded rather than unbounded: a
-- migration willing to wait forever stalls every write to deal and contract
-- for exactly as long as somebody left a transaction open.
SET LOCAL lock_timeout = '3s';

ALTER TABLE contract DROP CONSTRAINT contract_arr_nonnegative;
ALTER TABLE contract DROP CONSTRAINT contract_money_currency_pair;
ALTER TABLE contract DROP COLUMN arr_minor;
ALTER TABLE contract ADD CONSTRAINT contract_value_pair
    CHECK (((value_minor IS NULL) = (currency IS NULL)));

ALTER TABLE deal DROP CONSTRAINT deal_expected_arr_nonnegative;
ALTER TABLE deal DROP CONSTRAINT deal_money_currency_pair;
ALTER TABLE deal DROP COLUMN expected_arr_minor;
ALTER TABLE deal ADD CONSTRAINT deal_amount_currency_pair
    CHECK (((amount_minor IS NULL) = (currency IS NULL)));
