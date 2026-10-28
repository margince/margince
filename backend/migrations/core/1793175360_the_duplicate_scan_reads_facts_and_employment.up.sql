SET LOCAL lock_timeout = '3s';
-- The nightly duplicate scan keeps a watermark per table it reads. A stated
-- phone number (company_fact) and a contact employed at a company
-- (relationship) are second facts a similar company name needs, so the scan
-- reads them too.
ALTER TABLE dedupe_scan_state DROP CONSTRAINT dedupe_scan_state_stream_check;
ALTER TABLE dedupe_scan_state ADD CONSTRAINT dedupe_scan_state_stream_check
    CHECK (stream IN ('company', 'company_domain', 'contact', 'contact_email', 'contact_phone',
                      'company_fact', 'relationship')) NOT VALID;
