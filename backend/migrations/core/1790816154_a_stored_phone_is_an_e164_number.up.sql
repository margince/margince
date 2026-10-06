-- contact_phone.phone carries an "E.164 normalized at write" contract and the
-- database did not hold it. Fourteen other normalised columns in this schema are
-- held by a CHECK; this was the only one whose rule lived solely in Go.
--
-- Phone normalisation is parsing, not case folding. `+49 30 1234`, `+49301234`,
-- `0049301234` and `030 1234` are four spellings of one number, and only the
-- E.164 form matches in the dedupe lane — so a row that reaches this table
-- unnormalised is not untidy, it is invisible to dedupe, and one contact is
-- created twice with nothing saying so.
--
-- The pattern is values.E164Pattern, not a second spelling of it: "+", a non-zero
-- country digit, 8–15 digits total. A database looser than the Go seam would
-- admit exactly the rows the seam exists to refuse, which is the divergence this
-- constraint is meant to close rather than relocate.
-- TestTheContactPhoneCheckIsThisPattern fails in both directions if they part.
-- NOT VALID here, and the scan is the next migration. contact_phone is a written
-- table, so a plain ADD CONSTRAINT would check every stored row while holding
-- ACCESS EXCLUSIVE — lock_timeout bounds how long a statement waits for a lock,
-- never how long it holds one.
SET LOCAL lock_timeout = '3s';

ALTER TABLE contact_phone
    ADD CONSTRAINT contact_phone_e164 CHECK (phone ~ '^\+[1-9][0-9]{7,14}$') NOT VALID;
