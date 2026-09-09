-- A stop carried onto a surviving record says which row it came from.
--
-- A merge copies the retiring subject's live stops onto the survivor rather
-- than repointing them: the original is evidence that THAT record's subject
-- objected, and rewriting its subject would make the history say the objection
-- was made about somebody else. The copy needs to name its origin, or the two
-- rows read as two independent objections and nobody can tell that one merge
-- produced both.
--
-- Nullable and unbackfilled on purpose. Every row written before this carry
-- existed was recorded directly by somebody, and stamping them with a
-- provenance they do not have would be inventing history to fill a column.
-- NULL means "recorded here", which is true of all of them.
--
-- ON DELETE SET NULL rather than CASCADE: an erasure that reaches the
-- predecessor must not take the survivor's stop with it. The survivor's own
-- objection outlives the record it was carried from, which is the entire point
-- of copying rather than moving.
ALTER TABLE communication_suppression
  ADD COLUMN carried_from uuid
    REFERENCES communication_suppression (id) ON DELETE SET NULL;

COMMENT ON COLUMN communication_suppression.carried_from IS
  'The stop this one was copied from when a merge or promotion retired its subject. NULL when recorded directly.';
