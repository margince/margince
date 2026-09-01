-- The Microsoft 365 calendar is a connection in its own right.
--
-- Outlook mail has been a standing connection since the Graph connector landed,
-- and Google's pair has always been two rows — a mailbox and a calendar, each
-- with its own consent, its own scope and its own disconnect. Microsoft's half
-- of that pair had nowhere to live: the provider check admitted `graph` and
-- nothing calendar-shaped, so a Microsoft calendar could not be stored at all.
--
-- A SECOND row rather than a widened `graph` one, mirroring gmail/gcal. One row
-- would mean one consent carrying both OAuth scopes, and then a person who wants
-- their calendar in the CRM but not their mail has no way to say so — and
-- disconnecting either would take the other with it.
--
-- Widening a CHECK is additive: every existing row already satisfies it, so the
-- constraint is replaced rather than validated over history.
SET LOCAL lock_timeout = '3s';

ALTER TABLE capture_connection DROP CONSTRAINT capture_connection_provider_check;

ALTER TABLE capture_connection
    ADD CONSTRAINT capture_connection_provider_check
    CHECK (provider IN ('gmail', 'gcal', 'imap', 'graph', 'graphcal', 'whatsapp', 'telegram', 'offline_demo'));
