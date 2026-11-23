-- Narrowing back would refuse a contact kept for a shared mailbox, so those
-- contacts keep their owner and read as decided by a human.
SET LOCAL lock_timeout = '3s';

UPDATE contact SET narrowing_reason = 'human_decided' WHERE narrowing_reason = 'shared_mailbox';
ALTER TABLE contact DROP CONSTRAINT contact_narrowing_reason_check;
ALTER TABLE contact ADD CONSTRAINT contact_narrowing_reason_check CHECK (narrowing_reason IN (
    'awaiting_verdict', 'outbound_no_answer', 'confidentiality_hold', 'advisor', 'human_decided')) NOT VALID;
