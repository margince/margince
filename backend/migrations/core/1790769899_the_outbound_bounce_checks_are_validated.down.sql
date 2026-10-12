-- Put all three back present and unvalidated. Postgres has no ALTER that
-- un-validates, so each is dropped and re-added NOT VALID, without a scan.
SET LOCAL lock_timeout = '3s';

ALTER TABLE comms_outbound
    DROP CONSTRAINT comms_outbound_bounce_is_stated,
    ADD CONSTRAINT comms_outbound_bounce_is_stated CHECK (
        (bounced_at IS NULL) = (bounce_kind IS NULL)
        AND (bounce_reason IS NULL OR bounced_at IS NOT NULL)) NOT VALID,
    DROP CONSTRAINT comms_outbound_bounce_kind_named,
    ADD CONSTRAINT comms_outbound_bounce_kind_named CHECK (
        bounce_kind IS NULL OR bounce_kind IN ('hard', 'soft')) NOT VALID,
    DROP CONSTRAINT comms_outbound_bounce_recipient_stated,
    ADD CONSTRAINT comms_outbound_bounce_recipient_stated CHECK (
        bounce_recipient IS NULL OR bounced_at IS NOT NULL) NOT VALID;
