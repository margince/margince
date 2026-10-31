-- Validate the two checks the migration before this one added NOT VALID.
--
-- Their own file because the runner wraps each file in one transaction. Both
-- statements here are VALIDATE, which scans under SHARE UPDATE EXCLUSIVE.
-- Brief writes then do not queue behind the previous file's ACCESS EXCLUSIVE.
--
-- Unlike its siblings, this scan can fail. The columns are not new, so a row
-- naming both a contact and a lead would refuse it.
--
-- No writer in the tree can have made such a row. consentproof.go and
-- consentcarry.go each choose a single subject column at the call, and
-- communication_basis and sdr_handoff have always enforced the same rule.
--
-- An installation with one anyway, from a hand-run statement or an import,
-- sees the scan refuse. That is the right answer for a table recording a legal
-- basis: it refuses rather than leaving a record with two subjects.
--
-- Find such a row by selecting from either table where contact_id and lead_id
-- are both not null. Which subject it belongs to is a question about that row,
-- so the answer is an operator's and not this migration's.
SET LOCAL lock_timeout = '3s';

ALTER TABLE consent_event VALIDATE CONSTRAINT consent_event_subject;
ALTER TABLE contact_consent VALIDATE CONSTRAINT contact_consent_subject;
