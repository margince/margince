-- Restores the release ledger's SHAPE. It cannot restore the releases
-- themselves: the snapshots were dropped, and no other row holds what a given
-- release froze. A room rolled back this far reads as never published, which
-- is the honest answer rather than a reconstructed one.
SET LOCAL lock_timeout = '5s';

ALTER TABLE deal_room ADD COLUMN IF NOT EXISTS published_at timestamptz;

CREATE TABLE IF NOT EXISTS deal_room_release (
    id uuid DEFAULT uuidv7() NOT NULL,
    room_id uuid NOT NULL,
    release_no integer NOT NULL,
    snapshot jsonb NOT NULL,
    release_note text,
    source text NOT NULL,
    captured_by text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT deal_room_release_pkey PRIMARY KEY (id),
    CONSTRAINT deal_room_release_room_fkey FOREIGN KEY (room_id)
        REFERENCES deal_room(id) ON DELETE CASCADE,
    CONSTRAINT uq_deal_room_release_no UNIQUE (room_id, release_no)
);

-- The immutability guard comes back with the table it guards.
CREATE OR REPLACE FUNCTION deal_room_release_is_frozen() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    IF TG_OP = 'UPDATE' THEN
        RAISE EXCEPTION 'deal_room_release is immutable: publish a new release instead of editing release % of room %', OLD.release_no, OLD.room_id;
    END IF;
    RAISE EXCEPTION 'deal_room_release is immutable: release % of room % goes only with its room', OLD.release_no, OLD.room_id;
END;
$$;

CREATE TRIGGER trg_deal_room_release_is_frozen
    BEFORE UPDATE OR DELETE ON deal_room_release
    FOR EACH ROW EXECUTE FUNCTION deal_room_release_is_frozen();
