SET LOCAL lock_timeout = '3s';

DROP INDEX IF EXISTS comms_outbound_controller_payload;

ALTER TABLE comms_outbound
    DROP CONSTRAINT IF EXISTS comms_outbound_inflight_cannot_detect_prior_send,
    DROP CONSTRAINT IF EXISTS comms_outbound_payload_expires,
    DROP CONSTRAINT IF EXISTS comms_outbound_payload_is_controllers,
    DROP CONSTRAINT IF EXISTS comms_outbound_template_is_controllers,
    DROP CONSTRAINT IF EXISTS comms_outbound_purpose_is_users,
    DROP CONSTRAINT IF EXISTS comms_outbound_sender_stated,
    DROP CONSTRAINT IF EXISTS comms_outbound_sender_kind;

-- Reversing the relaxations needs the controller rows gone: they are the rows
-- that cannot satisfy the old NOT NULLs.
DELETE FROM comms_outbound WHERE sender_kind = 'controller';

ALTER TABLE comms_outbound
    DROP COLUMN IF EXISTS link_id,
    DROP COLUMN IF EXISTS payload_expires_at,
    DROP COLUMN IF EXISTS payload_ref,
    DROP COLUMN IF EXISTS template_version,
    DROP COLUMN IF EXISTS template_key,
    DROP COLUMN IF EXISTS sender_kind;

ALTER TABLE comms_outbound ALTER COLUMN consent_purpose SET NOT NULL;
ALTER TABLE comms_outbound ALTER COLUMN user_id SET NOT NULL;

ALTER TABLE comms_outbound
    ADD CONSTRAINT comms_outbound_inflight_is_channel
        CHECK (inflight_at IS NULL OR channel_user_id IS NOT NULL);
