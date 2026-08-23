-- What a buyer did in a Deal Room: signed in, opened a document, downloaded it.
--
-- The seller can already see WHETHER a participant has been here
-- (deal_room_session.last_seen_at). This records WHAT they did, which is the
-- question a rep actually asks before a call: did they read the contract.
--
-- One row per act rather than a counter, because "they opened it three times
-- last week" and "they opened it once in March" are different facts about the
-- same deal, and a counter collapses them.
--
-- No workspace column: every row hangs off deal_room_participant, which hangs
-- off deal_room, which is scoped by its deal. The cascade is what deletes it.
SET LOCAL lock_timeout = '5s';

-- The target a room-scoped child binds to when it must prove its document
-- belongs to the SAME room it names, exactly as deal_room_participant already
-- carries for its own children. Not redundant with the primary key.
ALTER TABLE deal_room_document
    ADD CONSTRAINT uq_deal_room_document_in_room UNIQUE (id, room_id);

CREATE TABLE deal_room_engagement (
    id uuid DEFAULT uuidv7() NOT NULL,
    room_id uuid NOT NULL,
    participant_id uuid NOT NULL,
    -- The document acted on, when the act names one. A sign-in names none.
    document_id uuid,
    kind text NOT NULL,
    occurred_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT deal_room_engagement_pkey PRIMARY KEY (id),
    CONSTRAINT deal_room_engagement_room_fkey FOREIGN KEY (room_id)
        REFERENCES deal_room(id) ON DELETE CASCADE,
    -- The pair binds the participant to the SAME room the row names, so a seat
    -- from another room can never be recorded as active in this one.
    CONSTRAINT deal_room_engagement_participant_fkey FOREIGN KEY (participant_id, room_id)
        REFERENCES deal_room_participant(id, room_id) ON DELETE CASCADE,
    -- The pair, like the participant above: a row can never record an act on
    -- a document belonging to another room. RESTRICT rather than SET NULL,
    -- because nulling document_id would violate the kind pairing below and
    -- fail the delete anyway — two constraints that contradict each other are
    -- a delete that fails for a reason nobody can read.
    CONSTRAINT deal_room_engagement_document_fkey FOREIGN KEY (document_id, room_id)
        REFERENCES deal_room_document(id, room_id) ON DELETE RESTRICT,
    CONSTRAINT deal_room_engagement_kind_check
        CHECK (kind IN ('signed_in', 'document_downloaded')),
    -- A sign-in is about the room; a download is about a document. Recording
    -- one shaped like the other would make the read below lie about which.
    CONSTRAINT deal_room_engagement_document_matches_kind
        CHECK ((kind = 'document_downloaded') = (document_id IS NOT NULL))
);

-- The read this table exists for: one participant's acts, newest first.
CREATE INDEX ix_deal_room_engagement_participant
    ON deal_room_engagement (participant_id, occurred_at DESC);

-- The room-wide read the Access panel does in one query.
CREATE INDEX ix_deal_room_engagement_room
    ON deal_room_engagement (room_id, occurred_at DESC);
