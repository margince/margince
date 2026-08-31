-- Two sender kinds the verdict engine had no word for: the mailbox owner's
-- private correspondents, and the professionals they engage personally.
--
-- Without them every family letter and every message from a founder's lawyer
-- had to be answered with one of the six business kinds. `person` published
-- them to the workspace; `spam` and `newsletter` were false about mail the
-- owner wanted. The engine picked one anyway, because the vocabulary is closed
-- and a model must answer from it.
--
-- The lifecycle statuses are unchanged: `personal` resolves to noise and
-- `advisor` to real, both already permitted by the status CHECK beside this one.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_pending_counterparty
    DROP CONSTRAINT IF EXISTS capture_pending_counterparty_kind_check;

ALTER TABLE capture_pending_counterparty
    ADD CONSTRAINT capture_pending_counterparty_kind_check
    CHECK (kind IS NULL OR kind IN (
        'person', 'role_mailbox', 'organization_sender',
        'newsletter', 'transactional', 'spam',
        'personal', 'advisor'));
