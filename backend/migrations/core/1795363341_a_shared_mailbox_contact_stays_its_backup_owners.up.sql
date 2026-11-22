-- A contact a verdict mints off mail only a shared mailbox captured stays its
-- backup owner's. Widening only, so NOT VALID here and validated in the file
-- beside this one, so no write waits on a scan under ACCESS EXCLUSIVE.
SET LOCAL lock_timeout = '3s';

ALTER TABLE contact DROP CONSTRAINT contact_narrowing_reason_check;
ALTER TABLE contact ADD CONSTRAINT contact_narrowing_reason_check CHECK (narrowing_reason IN (
    'awaiting_verdict', 'outbound_no_answer', 'confidentiality_hold', 'advisor', 'human_decided',
    'shared_mailbox')) NOT VALID;
