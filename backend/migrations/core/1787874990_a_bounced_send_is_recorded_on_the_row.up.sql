-- A sent message the receiving system returned is recorded on its own row.
--
-- Until now a bounce was invisible end to end: the capture side filtered the
-- delivery report as machine noise, and the outbound row stayed 'sent' — the
-- one status that reads as success. A rep whose mail never arrived had no way
-- to learn it short of the customer telling them.
--
-- bounced_at and bounce_kind make the fact durable and queryable. kind is the
-- half a consumer acts on: 'hard' means the address does not accept mail and
-- retrying is sending to nobody; 'soft' is a temporary refusal (full mailbox,
-- greylisting) that says nothing durable about the address. bounce_reason is
-- the report's stated reason, bounded upstream and safe to show an operator.
--
-- status stays 'sent' on purpose: the provider DID accept and dispatch the
-- message, every status reader (the dispatcher's terminal guard, the retry
-- ladder) depends on that fact, and the bounce is a later fact ABOUT the send,
-- not a different outcome of it.
SET LOCAL lock_timeout = '3s';

ALTER TABLE comms_outbound
    ADD COLUMN IF NOT EXISTS bounced_at timestamptz,
    ADD COLUMN IF NOT EXISTS bounce_kind text,
    ADD COLUMN IF NOT EXISTS bounce_reason text;

-- A bounce has a time and a kind, or it is not a bounce. reason may be empty —
-- many reports carry none — but never present without the fact it explains.
ALTER TABLE comms_outbound
    ADD CONSTRAINT comms_outbound_bounce_is_stated
    CHECK (((bounced_at IS NULL) = (bounce_kind IS NULL))
           AND (bounce_reason IS NULL OR bounced_at IS NOT NULL)) NOT VALID;

ALTER TABLE comms_outbound
    ADD CONSTRAINT comms_outbound_bounce_kind_named
    CHECK (bounce_kind IS NULL OR bounce_kind IN ('hard', 'soft')) NOT VALID;

-- Readers ask for the bounced rows and nothing else, so the index carries
-- only those.
CREATE INDEX IF NOT EXISTS comms_outbound_bounced_idx
    ON comms_outbound (bounced_at DESC)
    WHERE bounced_at IS NOT NULL;
