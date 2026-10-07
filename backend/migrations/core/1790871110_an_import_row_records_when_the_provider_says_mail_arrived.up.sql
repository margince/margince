SET LOCAL lock_timeout = '3s';
-- When the mailbox PROVIDER says a message arrived in this seat's mailbox:
-- Gmail's internalDate, Graph's receivedDateTime. Capture fills it from the
-- provider's API response, not from the message, so a sender cannot backdate
-- it the way they can write any Date header. NULL when the transport stated
-- none, and NULL proves nothing.
--
-- On its own in this migration so the ACCESS EXCLUSIVE lock the new column
-- takes is released at once. Filling it for older mail is the next
-- migration's work, under row locks only.
ALTER TABLE capture_import ADD COLUMN IF NOT EXISTS provider_received_at timestamptz;
