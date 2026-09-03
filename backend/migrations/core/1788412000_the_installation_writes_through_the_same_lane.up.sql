-- The installation's own mail rides the durable lane instead of a direct SMTP
-- call.
--
-- Product-originated mail — the confirm-details link, and the notices that
-- follow it — went out through platform/mailer: a synchronous SMTP send with no
-- delivery row, no retry ladder, no bounce handling and no record that it was
-- attempted. A relay outage lost the message and the token with it, and the
-- response called a relay's acceptance "delivered", which is a claim about a
-- mailbox nobody had heard from.
--
-- comms_outbound already carries every one of those properties for user mail.
-- So the installation becomes a second KIND of sender on the same table rather
-- than a second table with a second dispatcher: one row shape, one job kind,
-- one park-and-retry ladder, one bounce path.

SET LOCAL lock_timeout = '3s';

ALTER TABLE comms_outbound
    ADD COLUMN sender_kind text NOT NULL DEFAULT 'user',
    -- Which registered template rendered this message. A controller send may
    -- carry no arbitrary subject and body: the installation writes in its own
    -- name, so what it says is fixed in code and versioned, never composed by
    -- a caller.
    ADD COLUMN template_key text,
    ADD COLUMN template_version integer,
    -- The one-time link material, held in the key vault and referenced here.
    -- The plaintext never lands in a row: it is fetched at dispatch, rendered
    -- into the body in memory, and the reference is cleared once the relay has
    -- taken the message or the delivery reaches a terminal state.
    ADD COLUMN payload_ref text,
    ADD COLUMN payload_expires_at timestamptz,
    ADD COLUMN link_id uuid REFERENCES confirm_token(id) ON DELETE SET NULL;

-- user_id stops being mandatory, because a controller message is not sent BY
-- anybody: there is no seat to check, no mailbox grant to intersect and no
-- human whose deactivation should stop it. The CHECK keeps the two kinds
-- honest — a user row still names its user, and a controller row still may not.
ALTER TABLE comms_outbound ALTER COLUMN user_id DROP NOT NULL;

-- Same for the consent purpose. A controller message is authorized by its
-- category and its registered template, not by a purpose key a caller chose,
-- and leaving the column mandatory would force every controller row to borrow
-- 'transactional' — the very unconditional-allow class this work removes.
ALTER TABLE comms_outbound ALTER COLUMN consent_purpose DROP NOT NULL;

ALTER TABLE comms_outbound
    ADD CONSTRAINT comms_outbound_sender_kind
        CHECK (sender_kind = ANY (ARRAY['user'::text, 'controller'::text])),
    ADD CONSTRAINT comms_outbound_sender_stated
        CHECK ((sender_kind = 'user') = (user_id IS NOT NULL)),
    ADD CONSTRAINT comms_outbound_purpose_is_users
        CHECK ((sender_kind = 'user') = (consent_purpose IS NOT NULL)),
    ADD CONSTRAINT comms_outbound_template_is_controllers
        CHECK ((sender_kind = 'controller')
               = (template_key IS NOT NULL AND template_version IS NOT NULL)),
    ADD CONSTRAINT comms_outbound_payload_is_controllers
        CHECK (payload_ref IS NULL OR sender_kind = 'controller'),
    ADD CONSTRAINT comms_outbound_payload_expires
        CHECK ((payload_ref IS NULL) OR (payload_expires_at IS NOT NULL));

-- The at-most-once marker was refused on a mail-shaped row because an IMAP or
-- Gmail mailbox can be re-read to discover a prior send. An SMTP relay cannot:
-- it returns once and remembers nothing, so a controller row will need the
-- marker for exactly the reason a channel row does.
--
-- The constraint is relaxed here and the guard is armed with the transport that
-- sends these rows: resolveSeam still takes the mail arm for them today, which
-- declares the provider able to detect a prior send. Nothing produces a
-- controller row yet, so no send can double — but the transport must set
-- detectsPriorSend false for this sender, or a delivery whose outcome is
-- unknown goes back on the ladder and can send twice.
ALTER TABLE comms_outbound DROP CONSTRAINT comms_outbound_inflight_is_channel;
ALTER TABLE comms_outbound
    ADD CONSTRAINT comms_outbound_inflight_cannot_detect_prior_send
        CHECK (inflight_at IS NULL
               OR channel_user_id IS NOT NULL
               OR sender_kind = 'controller');

-- The sweep reads this: a reference whose link has expired is material nobody
-- can use and nothing will clear on its own.
CREATE INDEX comms_outbound_controller_payload
    ON comms_outbound (payload_expires_at) WHERE payload_ref IS NOT NULL;
