-- Put the check back present and unvalidated, as this migration found it.
-- Postgres has no ALTER that un-validates, so it is dropped and re-added.
SET LOCAL lock_timeout = '3s';

ALTER TABLE dedupe_scan_state
    DROP CONSTRAINT dedupe_scan_state_stream_check,
    ADD CONSTRAINT dedupe_scan_state_stream_check
        CHECK (stream IN ('company', 'company_domain', 'contact', 'contact_email', 'contact_phone',
                          'company_fact', 'relationship')) NOT VALID;
