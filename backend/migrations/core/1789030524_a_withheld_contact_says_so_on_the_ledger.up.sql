-- A `person` verdict does not always publish the contact.
--
-- Three situations keep the record visible to the mailbox owner alone: the
-- owner wrote first and the address has never answered, the thread is under a
-- confidentiality hold, and the message itself is restricted. That decision was
-- recorded only on the person row, and it did not survive there.
--
-- Two other readers ask the ledger the SAME question in the same words —
-- "status = 'real' AND kind = 'person'" — and treat the answer as permission:
-- the widening sweep reopens the mail a classified mailbox held, and the birth
-- decision shares a future message from a sender already judged a person. A
-- contact withheld in the moment was therefore republished by the next sweep,
-- and its correspondence shared by the next message.
--
-- So the withholding belongs HERE, beside the verdict both readers already
-- read, rather than on a row neither of them looks at.
-- Bounded so a long-running transaction holding a conflicting lock stalls
-- this migration rather than every write to the ledger behind it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_pending_counterparty
    ADD COLUMN IF NOT EXISTS withheld_from_workspace boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN capture_pending_counterparty.withheld_from_workspace IS
    'The verdict said person, and the contact was still kept to the mailbox owner. Every reader that treats a person verdict as permission to publish or share must exclude these.';
