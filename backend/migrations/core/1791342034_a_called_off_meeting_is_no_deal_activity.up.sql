-- A canceled or no-show meeting no longer keeps deal.last_activity_at moved
-- forward.
--
-- last_activity_of_deal folds the column and the activity triggers refold it,
-- but neither read meeting_status. A meeting dated after every other touch set
-- the deal's clock, and calling it off changed nothing, so deal health and the
-- stalled flag read the deal as recently active because of a meeting that did
-- not happen.
--
-- The added clause is relstrength.NotCalledOffSQL("a") verbatim, held there by
-- backend/gates/dealclockmeetingrule_test.go. A booked meeting still counts,
-- whenever it is dated, as it does for contact strength.
SET LOCAL lock_timeout = '3s';

CREATE OR REPLACE FUNCTION last_activity_of_deal(did uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(a.occurred_at)
    FROM activity_link l
    JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
     AND a.audience = 'workspace'
     AND a.origin NOT IN ('system_remediation', 'system_notice')
     AND (a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held')))
   WHERE l.deal_id = did
$$;

-- Calling a meeting off writes meeting_status alone, so the trigger must fire
-- on it, and on kind, which the clause reads too.
DROP TRIGGER activity_last_activity ON activity;
CREATE TRIGGER activity_last_activity
	AFTER UPDATE OF occurred_at, archived_at, audience, origin, kind, meeting_status ON activity
	FOR EACH ROW
	WHEN (old.occurred_at IS DISTINCT FROM new.occurred_at
	   OR old.archived_at IS DISTINCT FROM new.archived_at
	   OR old.audience IS DISTINCT FROM new.audience
	   OR old.origin IS DISTINCT FROM new.origin
	   OR old.kind IS DISTINCT FROM new.kind
	   OR old.meeting_status IS DISTINCT FROM new.meeting_status)
	EXECUTE FUNCTION trg_activity_last_activity();

-- Through move_last_activity, so a clock move bumps no version and stamps no
-- updated_at. Only deals whose value moves are written, in id order.
DO $$
DECLARE
  r record;
BEGIN
  FOR r IN
    SELECT id FROM deal
     WHERE last_activity_at IS DISTINCT FROM last_activity_of_deal(id)
     ORDER BY id
  LOOP
    PERFORM move_last_activity('deal'::regclass, r.id);
  END LOOP;
END $$;
