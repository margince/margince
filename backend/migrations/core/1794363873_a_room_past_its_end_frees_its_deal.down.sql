SET LOCAL lock_timeout = '3s';

CREATE UNIQUE INDEX uq_deal_room_active ON deal_room (deal_id)
    WHERE state IN ('draft', 'building', 'ready', 'publishing', 'live', 'paused')
      AND archived_at IS NULL;

ALTER TABLE deal_room DROP CONSTRAINT deal_room_one_open_room_per_deal;

DROP TRIGGER deal_room_steward_user_id_retirement_guard ON deal_room;
CREATE TRIGGER deal_room_steward_user_id_retirement_guard BEFORE INSERT OR UPDATE ON deal_room
    FOR EACH ROW WHEN (NEW.steward_user_id IS NOT NULL AND (NEW.archived_at IS NULL AND NEW.state NOT IN ('closed','expired')))
    EXECUTE FUNCTION refuse_retired_member_work('steward_user_id');
