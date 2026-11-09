-- pgmigrate:no-transaction
-- A captured echo of a sent message finds its delivery by the provider's own
-- message id, scoped to the sending seat and provider. Clear a failed
-- concurrent build before retrying without blocking delivery writes.
DROP INDEX CONCURRENTLY IF EXISTS comms_outbound_provider_message;
CREATE INDEX CONCURRENTLY comms_outbound_provider_message
    ON comms_outbound (user_id, provider, provider_message_id)
    WHERE provider_message_id IS NOT NULL;
