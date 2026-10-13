-- A seat's withdrawn address, kept only as long as its undo.
--
-- Removing an owner identity moves its whole row here in the same transaction,
-- keyed by its own id, so a restore can put back the original id, source and
-- created_at. The capture
-- store deletes a row ten minutes after its withdrawal: the value is a private
-- address, and an undo is the only reason to hold it.
SET LOCAL lock_timeout = '3s';

CREATE TABLE capture_owner_identity_withdrawn (
    id uuid PRIMARY KEY,
    -- The withdrawal's own audit row: a later row about the identity means it
    -- moved on, and the restore names this one as the entry it reverses.
    audit_id uuid NOT NULL,
    -- The principal that withdrew it, who alone may put it back.
    withdrawn_by text NOT NULL,
    user_id uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
    kind text NOT NULL,
    value text NOT NULL,
    source text NOT NULL,
    created_by text NOT NULL,
    created_at timestamptz NOT NULL,
    withdrawn_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT capture_owner_identity_withdrawn_kind_check CHECK (kind IN ('address', 'domain')),
    CONSTRAINT capture_owner_identity_withdrawn_source_check
        CHECK (source IN ('user', 'provider', 'delivered_to')),
    CONSTRAINT capture_owner_identity_withdrawn_value_check CHECK ((value = lower(value)) AND (value <> ''))
);

CREATE INDEX capture_owner_identity_withdrawn_at_idx ON capture_owner_identity_withdrawn (withdrawn_at);
CREATE INDEX capture_owner_identity_withdrawn_user_idx ON capture_owner_identity_withdrawn (user_id);

-- A learned alias the seat withdrew. Discovery reads it before recording a
-- sighting, so the withdrawal outlasts the undo window: the sightings that
-- learned the address are still stored and would claim it again on the next
-- delivery.
CREATE TABLE capture_alias_refused (
    user_id uuid NOT NULL REFERENCES app_user(id) ON DELETE CASCADE,
    value text NOT NULL,
    refused_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, value),
    CONSTRAINT capture_alias_refused_value_check CHECK ((value = lower(value)) AND (value <> ''))
);
