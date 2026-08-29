-- The currency the run's revenue norm is in, stored beside the figure.
--
-- revenue_norm_minor is the base value the revenue factor measured every deal
-- against, and a proportion is only checkable against a NAMED base. Without the
-- currency a screen can print a bare number — which is not money, and reads as
-- whatever currency the reader assumes — or say nothing, which is what the home
-- screen does today.
--
-- Stored rather than read at serve time. The installation's base currency can
-- still be changed until deals freeze it, and a figure computed against EUR
-- captioned with the USD in force a week later would be a wrong reading offered
-- with confidence. The run says what it measured against, the way it already
-- says how many candidates it saw.
--
-- Empty means a run written before this column existed. A brief is one row per
-- rep per local day, so those age out within a day; the wire omits the field
-- rather than inventing a currency for them.
SET LOCAL lock_timeout = '3s';

ALTER TABLE brief_run
    ADD COLUMN revenue_norm_currency text DEFAULT '' NOT NULL,
    ADD CONSTRAINT brief_run_revenue_norm_currency_check
        CHECK (revenue_norm_currency = '' OR revenue_norm_currency ~ '^[A-Z]{3}$');
