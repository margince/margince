-- A notice that names a record says WHICH one, durably.
--
-- "A deal you own changed stage" is true of every deal a rep owns. The row
-- carried the sentence and nothing else, so the Worklist drew a line a reader
-- could not act on: they knew something moved and had to go find it.
--
-- The target is stored rather than derived at read time for the reason the
-- subject is: a notice is the durable half of delivery, and re-deriving what it
-- was about would depend on the automation still existing, still firing on the
-- same entity, and still meaning the same thing by it.
SET LOCAL lock_timeout = '3s';
ALTER TABLE notice
  ADD COLUMN target_type text,
  ADD COLUMN target_id uuid,
  -- Both or neither. A type with no id names a KIND of thing and cannot be
  -- opened; an id with no type cannot be routed to a screen. Either half alone
  -- is a link the client has to guess at, and a guessed link is the dead
  -- control this column exists to remove.
  ADD CONSTRAINT notice_target_paired
    CHECK ((target_type IS NULL) = (target_id IS NULL));
