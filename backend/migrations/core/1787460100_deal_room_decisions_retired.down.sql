-- Restores the decision ledger's SHAPE and the third capability.
--
-- It cannot restore the decisions themselves: the rows were dropped, and no
-- other table holds what a buyer confirmed. A room rolled back this far reads
-- as one where nobody ever decided, which is the honest answer rather than a
-- reconstructed one. The audit spine still records that the decisions happened.
--
-- The participants collapsed to `comment` are NOT sent back to `reviewer`
-- either: after the rollback they read as commenters, which is the authority
-- they have actually been exercising since the up migration ran.
SET LOCAL lock_timeout = '5s';

ALTER TABLE deal_room_participant
    DROP CONSTRAINT deal_room_participant_capability_check;

ALTER TABLE deal_room_participant
    ADD CONSTRAINT deal_room_participant_capability_check
        CHECK (capability IN ('view', 'comment', 'reviewer'));

CREATE TABLE IF NOT EXISTS deal_room_decision (
    id uuid DEFAULT uuidv7() NOT NULL,
    room_id uuid NOT NULL,
    document_id uuid NOT NULL,
    attachment_id uuid NOT NULL,
    participant_id uuid NOT NULL,
    kind text NOT NULL,
    note text,
    source text NOT NULL,
    captured_by text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT deal_room_decision_pkey PRIMARY KEY (id),
    CONSTRAINT deal_room_decision_room_fkey FOREIGN KEY (room_id)
        REFERENCES deal_room(id) ON DELETE CASCADE,
    CONSTRAINT deal_room_decision_document_fkey FOREIGN KEY (document_id)
        REFERENCES deal_room_document(id) ON DELETE CASCADE,
    CONSTRAINT deal_room_decision_attachment_fkey FOREIGN KEY (attachment_id)
        REFERENCES attachment(id) ON DELETE RESTRICT,
    CONSTRAINT deal_room_decision_participant_in_room
        FOREIGN KEY (participant_id, room_id)
        REFERENCES deal_room_participant(id, room_id) ON DELETE RESTRICT,
    CONSTRAINT deal_room_decision_kind_check CHECK (kind IN ('request_changes', 'confirm_version'))
);

CREATE INDEX IF NOT EXISTS idx_deal_room_decision_document
    ON deal_room_decision (document_id, created_at DESC);
