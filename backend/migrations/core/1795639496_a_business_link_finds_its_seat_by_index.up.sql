-- 1795620143: a_business_link_finds_its_seat_by_index.
SET LOCAL lock_timeout = '3s';

-- A seat cannot be deleted while a link names it, and the foreign key checks
-- that with a lookup on seat_id. The partial unique index beside it covers live
-- links only, which that lookup cannot use.
CREATE INDEX IF NOT EXISTS channel_connection_seat_idx ON channel_connection (seat_id);
