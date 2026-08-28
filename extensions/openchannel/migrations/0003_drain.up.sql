-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion
--
-- What the queue needs once something ACTS on it, and the ledger of what this
-- connector sent outward.
--
-- Two facts the receiving half had no use for. A drained row has to name the
-- timeline entry it became, or nothing can take that entry away again when the
-- activity is archived; and a row whose entry has been taken away is neither
-- pending nor ingested, so the state vocabulary grows the word for it. The
-- outbound table is the other direction's evidence: this connector posts to an
-- address a member registered, and an attempt that left is a fact somebody asks
-- about afterwards.

-- The timeline entry this row became, NULL until it becomes one and NULL
-- forever for a row that never will.
--
-- It is the join the archive subscription reads: an activity.archived event
-- carries an id and nothing about this unit, so the id is the only handle back
-- to the request that produced it.
ALTER TABLE ext.ext_openchannel_inbound ADD COLUMN activity_id uuid;

-- `withdrawn` joins the vocabulary: the entry this row landed as has been
-- archived, so what the row records happened and is no longer on any timeline.
--
-- It is a fourth state rather than a return to `pending`, which would have the
-- drain land the same message again the moment somebody archived it.
ALTER TABLE ext.ext_openchannel_inbound DROP CONSTRAINT ext_openchannel_inbound_state_check;
ALTER TABLE ext.ext_openchannel_inbound ADD CONSTRAINT ext_openchannel_inbound_state_check
    CHECK (state IN ('pending', 'ingested', 'failed', 'withdrawn'));

-- The drain's own two questions, neither of which the per-endpoint index can
-- answer: what is waiting anywhere in this installation, oldest first, and what
-- has been decided about for long enough to delete. Both are keyed on state and
-- ordered by arrival, so one index serves them.
CREATE INDEX ext_openchannel_inbound_state_idx
    ON ext.ext_openchannel_inbound (state, received_at);

-- Partial, because the only reader is the archive subscription and it always
-- arrives with an id: a row that landed nothing is not a row that event can
-- name, and indexing those would be indexing the majority of the table for a
-- lookup that never matches them.
CREATE INDEX ext_openchannel_inbound_activity_idx
    ON ext.ext_openchannel_inbound (activity_id) WHERE activity_id IS NOT NULL;

-- One row per outbound attempt: what this connector posted to a member's
-- registered address, and what came back.
--
-- IT IS NOT A QUEUE AND NOT A RETRY LADDER. The product stages a delivery and
-- retries it on its own ladder, upstream of this unit entirely; a second one
-- here would be a second answer to one question. What this table holds is the
-- EVIDENCE of attempts that already happened, which is what a member reading
-- the screen is asking for when a message did not arrive.
CREATE TABLE ext.ext_openchannel_outbound (
    id              uuid        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,

    -- The endpoint whose registered address was posted to. Inside this unit's
    -- own schema, where the restricted role holds everything, so unlike the
    -- owner below it is a constraint that can actually be declared.
    endpoint_id     uuid        NOT NULL
                    REFERENCES ext.ext_openchannel_endpoint (id) ON DELETE CASCADE,

    -- Whose credential signed it, copied from the endpoint row at insert. Frozen
    -- here for the reason the queue's copy is frozen: this is whose secret the
    -- receiver verified against, and a later change of ownership must not
    -- re-attribute a message that has already left.
    user_id         uuid        NOT NULL,

    -- The delivery the product staged, and the attempt number within it. The key
    -- is the product's own id for the delivery and is stable across attempts,
    -- which is what makes it usable as the signature nonce a receiver
    -- deduplicates on.
    delivery_key    text        NOT NULL CHECK (length(delivery_key) > 0),
    attempt         integer     NOT NULL CHECK (attempt > 0),

    -- Who it was addressed to, as the product's own channel identity names them:
    -- the account id, never a handle, because a handle can be released and
    -- re-claimed.
    recipient       text        NOT NULL CHECK (length(recipient) > 0),

    -- What became of it. `sent` is a receiver that accepted it, `refused` is one
    -- that answered and said no, and `unknown` is the request that went out with
    -- no usable answer coming back — the outcome no later attempt can settle,
    -- and the reason this column has three values rather than a boolean.
    outcome         text        NOT NULL
                    CHECK (outcome IN ('sent', 'refused', 'unknown')),

    -- Why, in this unit's own vocabulary and never the receiver's prose: the
    -- address belongs to a member's own system, and what it says about itself is
    -- not this installation's to store or to render.
    error_class     text,

    created_at      timestamptz NOT NULL DEFAULT now(),

    -- One row per attempt of one delivery. A repeat under the same pair is the
    -- same attempt being recorded twice, which would show a member two sends
    -- where one left.
    CONSTRAINT ext_openchannel_outbound_one_per_attempt
        UNIQUE (endpoint_id, delivery_key, attempt)
);

-- Newest first for one endpoint, which is the only way this table is read.
CREATE INDEX ext_openchannel_outbound_recent_idx
    ON ext.ext_openchannel_outbound (endpoint_id, created_at DESC);

-- The app role runs the unit's handlers. TRUNCATE is deliberately absent, for
-- the reason 0001 states.
GRANT SELECT, INSERT, UPDATE, DELETE ON ext.ext_openchannel_outbound TO margince_app;
