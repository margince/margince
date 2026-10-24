SET LOCAL lock_timeout = '3s';

-- deal_forecast_history is written on every amount and close-date change and
-- nothing reads it.
--
-- audit_log already records the same changes, with before and after values and
-- the actor, and sits in preservedResetTables with no retention path deleting
-- it. So the trail outlives these rows rather than duplicating them for a
-- while, and a future "this deal slipped twice" view is built from there. A
-- typed table can come back when that feature is designed, with its reader.
DROP TABLE deal_forecast_history;
