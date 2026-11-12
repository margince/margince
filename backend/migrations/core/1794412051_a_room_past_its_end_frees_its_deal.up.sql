-- A Deal Room whose access end has passed reads as expired, to staff and to
-- buyers alike. Two database rules still treated such a room as open.
--
-- First, the one-open-room-per-deal rule. A partial index cannot name now(),
-- so the rule moves into an exclusion over each room's access window. A new
-- room's window starts at its creation, so it clashes only with a room whose
-- end has not passed. Pushing the old room's end past that moment clashes too,
-- which keeps a deal at one open room.
--
-- Second, a steward of a room past its end no longer counts as active work.
SET LOCAL lock_timeout = '3s';

ALTER TABLE deal_room
    ADD CONSTRAINT deal_room_one_open_room_per_deal
    EXCLUDE USING gist (
        deal_id WITH =,
        tstzrange(created_at, expires_at) WITH &&)
    WHERE (state IN ('draft', 'building', 'ready', 'publishing', 'live', 'paused')
           AND archived_at IS NULL);

DROP INDEX uq_deal_room_active;

DROP TRIGGER deal_room_steward_user_id_retirement_guard ON deal_room;
CREATE TRIGGER deal_room_steward_user_id_retirement_guard BEFORE INSERT OR UPDATE ON deal_room
    FOR EACH ROW WHEN (NEW.steward_user_id IS NOT NULL AND (NEW.archived_at IS NULL AND NEW.state NOT IN ('closed','expired') AND (NEW.expires_at IS NULL OR NEW.expires_at > now())))
    EXECUTE FUNCTION refuse_retired_member_work('steward_user_id');
