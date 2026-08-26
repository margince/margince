-- Bounded: dropping this table takes a lock on app_user and passport through
-- its foreign keys, so an open transaction holding a conflicting lock would
-- otherwise stall every write to both for as long as this is willing to wait.
SET LOCAL lock_timeout = '3s';

DROP TABLE IF EXISTS agent_standing_grant;
