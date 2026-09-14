-- A resolved proposal says why.
--
-- contact_confirm_submission has carried resolution, resolved_at and
-- resolved_by since it shipped, and nothing ever wrote them: a subject could
-- propose a correction through their own link and no colleague could see it or
-- act on it. The queue that decides lands with this column.
--
-- The NOTE is what a decline needs most. An accepted correction explains itself
-- — the field now holds what the subject said — but "we did not change it"
-- records no reason at all, and the subject who asked is entitled to one when
-- they ask again.
SET LOCAL lock_timeout = '3s';

ALTER TABLE contact_confirm_submission
  ADD COLUMN note text;

-- Bounded here rather than only in the handler, because the column outlives
-- whichever door writes it. 500 is the same bound every other operator note in
-- this schema carries.
ALTER TABLE contact_confirm_submission
  ADD CONSTRAINT contact_confirm_submission_note_length
    CHECK (note IS NULL OR char_length(note) <= 500);

-- A note with no decision describes nothing: it is the reason for a resolution,
-- so it cannot arrive before one.
ALTER TABLE contact_confirm_submission
  ADD CONSTRAINT contact_confirm_submission_note_shape
    CHECK (note IS NULL OR resolution IS NOT NULL);
