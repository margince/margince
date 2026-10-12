-- The validation of the shape CHECKs the two previous migrations added. A file
-- of its own because VALIDATE takes a lighter lock than the ADD, and only a
-- separate transaction lets writers through while it scans. Each CHECK holds
-- for every row the table's type CHECK admits, so this cannot fail.
SET LOCAL lock_timeout = '3s';

ALTER TABLE record_grant VALIDATE CONSTRAINT record_grant_record_shape;
ALTER TABLE analytics_share VALIDATE CONSTRAINT analytics_share_scope_shape;
