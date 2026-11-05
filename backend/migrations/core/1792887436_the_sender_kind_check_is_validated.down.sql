-- Put the check back present and unvalidated. Postgres has no ALTER that
-- un-validates, so it is dropped and re-added NOT VALID, without a scan.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_pending_counterparty
    DROP CONSTRAINT capture_pending_counterparty_kind_check,
    ADD CONSTRAINT capture_pending_counterparty_kind_check
    CHECK (kind IS NULL OR kind IN (
        'contact', 'role_mailbox', 'company_sender',
        'newsletter', 'transactional', 'spam',
        'personal', 'advisor', 'vendor_pitch')) NOT VALID;
