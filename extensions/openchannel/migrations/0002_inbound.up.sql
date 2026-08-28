-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion
--
-- The received-but-not-yet-ingested queue: one row per signed request this
-- installation accepted on an anonymous edge, held until something acts on it.
--
-- Receiving and acting are two steps ON PURPOSE. The anonymous handler runs
-- with no authority at all — its principal is a bare connector with empty
-- permissions — so the only honest thing it can do is record what arrived, and
-- everything the payload eventually MEANS is decided later, under the owner's
-- own live authority. That is why the body is kept verbatim: it is the
-- evidence, and re-deriving it from a parsed shape would leave a signature
-- nothing can be re-checked against.

CREATE TABLE ext.ext_openchannel_inbound (
    id              uuid        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,

    -- The edge it arrived on. The foreign key is INSIDE this unit's own
    -- schema, where the restricted ext_openchannel role holds everything, so
    -- unlike the owner below it is a constraint that can actually be declared.
    -- ON DELETE CASCADE because a queue entry for an endpoint that no longer
    -- exists names nothing anyone can act for.
    endpoint_id     uuid        NOT NULL
                    REFERENCES ext.ext_openchannel_endpoint (id) ON DELETE CASCADE,

    -- The owner, COPIED from the endpoint row at insert and never read off the
    -- request. It is duplicated here rather than joined for on purpose: this is
    -- whose authority the payload will be acted under, and freezing it at the
    -- moment of arrival means a later change of ownership cannot silently
    -- re-attribute requests that were already accepted.
    --
    -- No foreign key, for the reason 0001 gives about the core user table.
    user_id         uuid        NOT NULL,

    -- The caller-chosen value the signature covers, and this row's idempotency
    -- key. UNIQUE per endpoint below: that pairing is what makes a correctly
    -- signed request landable exactly once, so a redelivery is a no-op insert
    -- rather than a second copy of one event.
    nonce           text        NOT NULL CHECK (length(nonce) > 0),

    -- The request body exactly as it was signed. bytea rather than text or
    -- jsonb because the signature covers BYTES: a round trip through a parsed
    -- representation would re-encode them, and the stored evidence would no
    -- longer verify against the signature that admitted it.
    body            bytea       NOT NULL,

    -- The instant the SENDER signed, as distinct from received_at below, which
    -- is this installation's own clock. Keeping both is what lets a reader see
    -- a sender whose clock is drifting rather than infer one.
    sent_at         timestamptz NOT NULL,

    -- Where this entry is in the queue. `pending` is waiting to be acted on,
    -- `ingested` has been, and `failed` is one nothing will retry — the class
    -- beside it says why, in this unit's own vocabulary and never a remote
    -- party's prose.
    state           text        NOT NULL DEFAULT 'pending'
                    CHECK (state IN ('pending', 'ingested', 'failed')),
    attempts        integer     NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    last_error_class text,

    received_at     timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT ext_openchannel_inbound_one_per_nonce UNIQUE (endpoint_id, nonce)
);

-- The two questions asked of this table on the hot path: how much is still
-- waiting for one endpoint (the bounded-queue check every accepted request
-- makes), and which entry to take next. Both are answered by this index, and
-- both are per endpoint, so a busy edge does not make a quiet one's read scan
-- its rows.
CREATE INDEX ext_openchannel_inbound_pending_idx
    ON ext.ext_openchannel_inbound (endpoint_id, state, received_at);

-- The app role runs the unit's handlers. TRUNCATE is deliberately absent, for
-- the reason 0001 states.
DO $$
BEGIN
  IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'margince_app') THEN
    GRANT SELECT, INSERT, UPDATE, DELETE ON ext.ext_openchannel_inbound TO margince_app;
  END IF;
END $$;
