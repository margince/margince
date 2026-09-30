-- Bounded because everything below blocks writers on a table this file did not
-- create: without a cap, an open transaction holding a conflicting lock stalls
-- every write to notice for as long as this is willing to queue, which is
-- forever. It bounds the WAIT and not the HOLD — the index rebuild still blocks
-- writers for as long as the build takes.
SET LOCAL lock_timeout = '3s';

-- A line that said a decision was waiting stops being true the moment somebody
-- else decides, and nothing took it back. overtaken_at records that it stopped;
-- overtaken_by records who, where a person did it and is recoverable.
--
-- No CHECK pairs the two. The foreign key below is ON DELETE SET NULL, and that
-- SET NULL is itself an update: a CHECK demanding a name would make deleting a
-- colleague fail against every line they ever overtook.
ALTER TABLE notice
  ADD COLUMN overtaken_at timestamptz,
  ADD COLUMN overtaken_by uuid;

ALTER TABLE notice
  ADD CONSTRAINT notice_overtaken_by_fkey
  FOREIGN KEY (overtaken_by) REFERENCES app_user(id) ON DELETE SET NULL;

-- The unread index carries the second arm, because every lane, badge and digest
-- read now asks both questions.
DROP INDEX notice_unread;
CREATE INDEX notice_unread ON notice (recipient_user_id, created_at DESC)
  WHERE read_at IS NULL AND overtaken_at IS NULL;
