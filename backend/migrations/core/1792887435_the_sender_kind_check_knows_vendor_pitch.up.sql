-- A sender kind for a named human who only pitches something to this business.
--
-- Until now the verdict engine had to answer `contact` for such a sender, which
-- filed the vendor as the rep's contact, or `spam`, which hides the mail. The
-- new kind is `real` in the lifecycle (the mail stays visible) and creates a
-- contact carrying the vendor_marketing relationship mark.
--
-- The constraint is replaced NOT VALID so no scan runs under the lock; the next
-- migration validates it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_pending_counterparty
    DROP CONSTRAINT IF EXISTS capture_pending_counterparty_kind_check;

ALTER TABLE capture_pending_counterparty
    ADD CONSTRAINT capture_pending_counterparty_kind_check
    CHECK (kind IS NULL OR kind IN (
        'contact', 'role_mailbox', 'company_sender',
        'newsletter', 'transactional', 'spam',
        'personal', 'advisor', 'vendor_pitch')) NOT VALID;
