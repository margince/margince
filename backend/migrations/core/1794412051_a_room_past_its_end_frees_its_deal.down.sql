SET LOCAL lock_timeout = '3s';

-- The unique index allows one open room per deal. Since the up migration, a
-- deal can hold an expired room beside a newer open one. Store the older ones
-- as expired, which is what they read as, so the index can come back.
UPDATE deal_room r
   SET state = 'expired'
 WHERE r.state IN ('draft', 'building', 'ready', 'publishing', 'live', 'paused')
   AND r.archived_at IS NULL
   AND EXISTS (
       SELECT 1 FROM deal_room newer
        WHERE newer.deal_id = r.deal_id
          AND newer.state IN ('draft', 'building', 'ready', 'publishing', 'live', 'paused')
          AND newer.archived_at IS NULL
          AND (newer.created_at, newer.id) > (r.created_at, r.id));

CREATE UNIQUE INDEX uq_deal_room_active ON deal_room (deal_id)
    WHERE state IN ('draft', 'building', 'ready', 'publishing', 'live', 'paused')
      AND archived_at IS NULL;

ALTER TABLE deal_room DROP CONSTRAINT deal_room_one_open_room_per_deal;

DROP TRIGGER deal_room_steward_user_id_retirement_guard ON deal_room;
CREATE TRIGGER deal_room_steward_user_id_retirement_guard BEFORE INSERT OR UPDATE ON deal_room
    FOR EACH ROW WHEN (NEW.steward_user_id IS NOT NULL AND (NEW.archived_at IS NULL AND NEW.state NOT IN ('closed','expired')))
    EXECUTE FUNCTION refuse_retired_member_work('steward_user_id');
