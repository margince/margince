-- A deal and a contract carry recurring value beside their one-off figure.
--
-- expected_arr_minor and arr_minor hold annual recurring revenue in the minor
-- units of the row's own currency. They are a SECOND money column on rows that
-- already have one, so the pairing CHECK that used to speak only of the
-- one-off amount has to speak of both: a currency is present exactly when at
-- least one figure is, and absent exactly when neither is.
--
-- Writing it as one equality rather than two implications keeps the failing
-- direction legible in the constraint name. The old constraints are dropped
-- and replaced rather than supplemented, because a row with ARR and no
-- one-off amount is legal under the new rule and illegal under the old one.

-- Both tables are live and written constantly, so the ALTERs below queue behind
-- any open transaction touching them. Bounded rather than unbounded: a
-- migration willing to wait forever stalls every write to deal and contract
-- for exactly as long as somebody left a transaction open.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal ADD COLUMN expected_arr_minor bigint;

ALTER TABLE deal DROP CONSTRAINT deal_amount_currency_pair;

ALTER TABLE deal ADD CONSTRAINT deal_money_currency_pair
    CHECK (((amount_minor IS NULL) AND (expected_arr_minor IS NULL)) = (currency IS NULL));

ALTER TABLE deal ADD CONSTRAINT deal_expected_arr_nonnegative
    CHECK (expected_arr_minor IS NULL OR expected_arr_minor >= 0);

COMMENT ON COLUMN deal.expected_arr_minor IS
    'Expected annual recurring revenue in minor units of deal.currency. Null means the deal carries no recurring component, which is different from zero.';

ALTER TABLE contract ADD COLUMN arr_minor bigint;

ALTER TABLE contract DROP CONSTRAINT contract_value_pair;

ALTER TABLE contract ADD CONSTRAINT contract_money_currency_pair
    CHECK (((value_minor IS NULL) AND (arr_minor IS NULL)) = (currency IS NULL));

ALTER TABLE contract ADD CONSTRAINT contract_arr_nonnegative
    CHECK (arr_minor IS NULL OR arr_minor >= 0);

COMMENT ON COLUMN contract.arr_minor IS
    'Annual recurring revenue in minor units of contract.currency. Null means the contract carries no recurring component, which is different from zero.';
