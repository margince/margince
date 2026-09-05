-- An address that DELIVERS into the connected mailbox is the owner's, not a
-- stranger's.
--
-- A seat's own address is known three ways: they declared it, their connection
-- reports it, or they sign in with it. None of those reaches a FORWARDING
-- ALIAS — an address that delivers into the mailbox but is never the From of
-- anything the mailbox sends, so no send-side discovery can see it. Measured on
-- a real mailbox: an alias appeared as a recipient on 24 captured messages and
-- became a contact. The founder was in his own CRM under that address.
--
-- The evidence is the receiving server's own Delivered-To, read only from a
-- position a sender could not have authored (mailmap.TopDeliveredTo). Getting
-- that backwards would let a sender declare themselves to be the mailbox owner,
-- so the ladder is deliberately slow: a sighting is recorded here first, and an
-- identity is written only once two DISTINCT messages carry the same claim.
--
-- One row per (seat, address, message). The unique key IS the distinctness
-- rule: a message re-synced, or replayed by a push notification, records the
-- same row rather than a second sighting, so the count cannot be inflated by
-- the same evidence arriving twice.
SET LOCAL lock_timeout = '3s';

CREATE TABLE capture_alias_sighting (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    user_id uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
    value text NOT NULL,
    -- The message this sighting came from, in the natural key's own spelling
    -- (`<system>:<id>`), so a re-sync of one message is one sighting.
    source text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    -- Folded, like every other address this module stores, so a lookup never
    -- has to guess which case the writer used.
    CONSTRAINT capture_alias_sighting_value_check CHECK ((value = lower(value)) AND (value <> '')),
    CONSTRAINT capture_alias_sighting_once_per_message UNIQUE (user_id, value, source)
);

-- 'delivered_to' is its own source and not 'user' or 'provider': the card has
-- to be able to say how the product learned this, and a seat looking at an
-- address they never typed deserves to see that it was inferred — and to
-- remove it.
ALTER TABLE capture_owner_identity
    DROP CONSTRAINT capture_owner_identity_source_check;

ALTER TABLE capture_owner_identity
    ADD CONSTRAINT capture_owner_identity_source_check
        CHECK (source IN ('user', 'provider', 'delivered_to'));
