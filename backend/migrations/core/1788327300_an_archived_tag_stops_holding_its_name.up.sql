-- An archived tag stops holding its name.
--
-- uq_tag_name has always covered every row, archived or not, so retiring a word
-- reserved it forever. That makes two product rules impossible rather than
-- merely unimplemented:
--
--   * merge releases the source's name — it does not, the archived row keeps it,
--     and recreating the word answers 409;
--   * restore refuses when a LIVE tag has taken the name meanwhile — no live tag
--     ever can, so the refusal is unreachable and the branch guarding it is dead.
--
-- The index becomes partial: uniqueness binds live rows only. Two archived rows
-- may now share a name, which is correct — they are history, and history is
-- allowed to repeat. Restoring one of them is what has to check, and it does.
--
-- Bounded: the index is rebuilt on a table the product writes whenever anybody
-- coins a tag.
SET LOCAL lock_timeout = '3s';

DROP INDEX uq_tag_name;

CREATE UNIQUE INDEX uq_tag_name ON tag USING btree (lower(name)) WHERE archived_at IS NULL;
