-- A staged proposal remembers what its target is CALLED.
--
-- Roughly half the stageable kinds carry no typed payload: their
-- proposed_change is the raw arguments a tool was called with, an automation
-- action's args, or a canonicalized HTTP body. Nothing in that bag names the
-- record, so the card falls back to the summary — and the summaries those paths
-- compose read like `automation wants to assign_owner on deal
-- 01a03781-9083-7565-8d65-5939ec0f3e70`. A rep asked to approve a change to a
-- record identified only by a uuid decides on the verb alone.
--
-- The name is resolved at STAGING time, in the transaction that writes the row,
-- because that is the last moment it is reliably in hand: the target may be
-- renamed, archived or merged before anyone opens the inbox, and a card that
-- re-resolved it then would name a record the approver was not shown.
--
-- Nullable, and nullable is meaningful: a kind whose target is a type with no
-- name, or a create with no target at all, has nothing to record — which is a
-- different fact from a name that was not looked up, and the read path leaves
-- the caption off rather than inventing one.
--
-- lock_timeout because `approval` is a pre-existing table: a deploy must fail
-- fast rather than queue behind a long reader.
SET LOCAL lock_timeout = '3s';

ALTER TABLE approval ADD COLUMN target_label text;
