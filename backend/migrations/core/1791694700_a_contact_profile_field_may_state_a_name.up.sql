-- A signature may state the contact's own name.
--
-- The display-name split is a guess: "Greven Ebusiness" parses to first
-- `Greven` and last `Ebusiness`, and every greeting then uses it. What the
-- conversation shows outranks the guess, and the statement is recorded here
-- beside every other field a signature states.
--
-- Widening only. Every existing row satisfies both the old vocabulary and this
-- one, so nothing needs rewriting. NOT VALID all the same, and validated in
-- the file beside this one: a plain ADD CONSTRAINT scans the whole table under
-- ACCESS EXCLUSIVE whether or not a row could fail, and every write waits for
-- the scan.
SET LOCAL lock_timeout = '3s';

ALTER TABLE contact_profile_field DROP CONSTRAINT contact_profile_field_field_check;
ALTER TABLE contact_profile_field ADD CONSTRAINT contact_profile_field_field_check
  CHECK (field IN ('title', 'phone', 'role', 'linkedin', 'company_name', 'address', 'website',
                   'first_name', 'last_name')) NOT VALID;
