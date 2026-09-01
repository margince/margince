-- Drop the disposition tables.
--
-- Every row here is a judgement somebody made, and rolling back discards them:
-- an installation that comes back will show every set-aside message again, as
-- if nobody had decided anything. That is the honest cost of removing the
-- feature, and it is why this is a DROP rather than an attempt to preserve
-- state a re-applied version could not read anyway.
SET LOCAL lock_timeout = '3s';
DROP TABLE IF EXISTS activity_reader_state;
DROP TABLE IF EXISTS activity_sales_state;
