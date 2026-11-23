-- A contact's impersonation verdict says which sign raised it and who cleared it.
--
-- quarantined_at is set when capture makes a contact from a sender that shows a
-- sign of impersonation. quarantine_reason names the sign as a closed code.
-- A verdict older than this column has no reason, and no existing row is changed.
--
-- A user who checks the sender clears the warning. The verdict stays, with the
-- time and the user (in the grammar of captured_by), as the record of the answer.
--
-- The constraints are added NOT VALID; the next migration validates them.
SET LOCAL lock_timeout = '3s';

ALTER TABLE contact
    ADD COLUMN IF NOT EXISTS quarantine_reason text,
    ADD COLUMN IF NOT EXISTS quarantine_cleared_at timestamptz,
    ADD COLUMN IF NOT EXISTS quarantine_cleared_by text,
    ADD CONSTRAINT contact_quarantine_reason_check CHECK (
        quarantine_reason = ANY (ARRAY['punycode_domain', 'display_name_other_domain'])) NOT VALID,
    ADD CONSTRAINT contact_quarantine_reason_needs_a_verdict CHECK (
        quarantine_reason IS NULL OR quarantined_at IS NOT NULL) NOT VALID,
    ADD CONSTRAINT contact_quarantine_cleared_needs_a_verdict CHECK (
        quarantine_cleared_at IS NULL OR quarantined_at IS NOT NULL) NOT VALID,
    ADD CONSTRAINT contact_quarantine_cleared_by_pairs CHECK (
        (quarantine_cleared_at IS NULL) = (quarantine_cleared_by IS NULL)) NOT VALID;

COMMENT ON COLUMN contact.quarantine_reason IS
    'Which sign of impersonation raised the verdict in quarantined_at: punycode_domain is a mail domain in punycode form, display_name_other_domain a display name that holds an address on another domain. NULL with a verdict means it was recorded before the reason was stored.';
COMMENT ON COLUMN contact.quarantine_cleared_at IS
    'When a user answered the verdict in quarantined_at as fine. The contact page shows the warning only while this is NULL.';
