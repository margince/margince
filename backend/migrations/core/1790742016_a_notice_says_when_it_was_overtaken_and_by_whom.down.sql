-- Reverses 1790742016. Bounded on the same terms as the up half: these block
-- writers on notice, and an unbounded wait behind a conflicting lock stalls
-- every write to it indefinitely.
SET LOCAL lock_timeout = '3s';

DROP INDEX notice_unread;
CREATE INDEX notice_unread ON notice (recipient_user_id, created_at DESC)
  WHERE read_at IS NULL;

ALTER TABLE notice DROP CONSTRAINT notice_overtaken_by_fkey;
ALTER TABLE notice
  DROP COLUMN overtaken_by,
  DROP COLUMN overtaken_at;
