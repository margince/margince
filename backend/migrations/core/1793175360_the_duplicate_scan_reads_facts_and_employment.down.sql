SET LOCAL lock_timeout = '3s';
DELETE FROM dedupe_scan_state WHERE stream IN ('company_fact', 'relationship');
ALTER TABLE dedupe_scan_state DROP CONSTRAINT dedupe_scan_state_stream_check;
ALTER TABLE dedupe_scan_state ADD CONSTRAINT dedupe_scan_state_stream_check
    CHECK (stream IN ('company', 'company_domain', 'contact', 'contact_email', 'contact_phone')) NOT VALID;
