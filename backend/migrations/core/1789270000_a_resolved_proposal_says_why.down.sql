SET LOCAL lock_timeout = '3s';

-- The constraints first: dropping a column a CHECK still names fails.
ALTER TABLE contact_confirm_submission
  DROP CONSTRAINT IF EXISTS contact_confirm_submission_note_shape,
  DROP CONSTRAINT IF EXISTS contact_confirm_submission_note_length;

ALTER TABLE contact_confirm_submission
  DROP COLUMN IF EXISTS note;
