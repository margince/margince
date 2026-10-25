SET LOCAL lock_timeout = '3s';

-- A vendor_pitch row falls back to `contact`, the kind that kept the same
-- lifecycle status. The rollback is lossy: the row no longer says the sender
-- was a vendor, and the mark on the contact it created stays.
UPDATE capture_pending_counterparty
   SET kind = 'contact' WHERE kind = 'vendor_pitch';

ALTER TABLE capture_pending_counterparty
    DROP CONSTRAINT IF EXISTS capture_pending_counterparty_kind_check;

ALTER TABLE capture_pending_counterparty
    ADD CONSTRAINT capture_pending_counterparty_kind_check
    CHECK (kind IS NULL OR kind IN (
        'contact', 'role_mailbox', 'company_sender',
        'newsletter', 'transactional', 'spam',
        'personal', 'advisor')) NOT VALID;
