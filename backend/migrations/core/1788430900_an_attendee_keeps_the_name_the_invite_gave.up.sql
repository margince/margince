-- The name a calendar invitation gave an attendee, kept on the row that records
-- their attendance.
--
-- A person minted from a bare address is named by its local part: an invite
-- addressed to `chris@example.org` and nothing else produced a contact called
-- "Chris", unconfident, first and last name null. The calendar event that
-- carried the invitation names them in full — Google and Microsoft both send
-- the attendee's display name — and every reader of that event dropped it.
--
-- Persisted rather than read back from the stored original each time, because
-- the two orderings need it at different moments: an attendee who is already a
-- contact is named as the meeting lands, while one who becomes a contact weeks
-- later is named by the cohort repair, long after the sync that saw the invite.
--
-- Three states, and the third is what a recovery pass selects on:
--   a name  — the provider named this attendee;
--   ''      — the stamp ran and the provider named nobody;
--   NULL    — the row was written before names were carried at all.
-- The backfill drains permanently because live stamping now always writes one
-- of the first two.
SET LOCAL lock_timeout = '5s';

ALTER TABLE activity_participant ADD COLUMN display_name text;

COMMENT ON COLUMN activity_participant.display_name IS
	'The name the provider gave this attendee: a name, '''' when it named nobody, NULL when the row predates the column. Subject data — cleared by the Art. 17 erasure and the retention anonymizer, and served by the Art. 15 export.';
