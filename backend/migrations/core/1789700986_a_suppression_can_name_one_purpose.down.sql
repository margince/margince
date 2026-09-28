-- The table is live: the engine reads it on every send. An unbounded ALTER
-- queues behind any open transaction and stalls every write for as long as it
-- is willing to wait, which is forever.
SET LOCAL lock_timeout = '3s';

ALTER TABLE communication_suppression DROP COLUMN purpose_id;
