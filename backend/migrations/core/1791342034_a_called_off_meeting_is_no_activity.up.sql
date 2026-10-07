-- A canceled or no-show meeting no longer keeps last_activity_at moved forward
-- on a deal, a contact or a company.
--
-- The last_activity_of_* functions fold the column and the activity triggers
-- refold it, but neither read meeting_status. A meeting dated after every other
-- touch set the record's clock, and calling it off changed nothing, so deal
-- health, the stalled flag and "last contacted" read the record as recently
-- active because of a meeting that did not happen.
--
-- The added clause is relstrength.NotCalledOffSQL("a") verbatim, in every arm,
-- held there by backend/gates/lastactivitymeetingrule_test.go. A booked meeting
-- still counts, whenever it is dated, as it does for contact strength.
--
-- Stored values are refolded by 1791342035, in its own transaction, so the
-- trigger swap's lock on activity is not held while every record is visited.
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

CREATE OR REPLACE FUNCTION last_activity_of_contact(cid uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(a.occurred_at)
    FROM activity_link l
    JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
     AND a.audience = 'workspace'
     AND a.origin NOT IN ('system_remediation', 'system_notice')
     AND (a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held')))
   WHERE l.contact_id = cid
$$;

CREATE OR REPLACE FUNCTION last_activity_of_company(cid uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(v) FROM (
    -- Filed against the account itself.
    SELECT max(a.occurred_at) AS v
      FROM activity_link l
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND a.origin NOT IN ('system_remediation', 'system_notice')
       AND (a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held')))
     WHERE l.company_id = cid
    UNION ALL
    -- Filed against one of its deals.
    SELECT max(a.occurred_at)
      FROM deal d
      JOIN activity_link l ON l.deal_id = d.id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND a.origin NOT IN ('system_remediation', 'system_notice')
       AND (a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held')))
     WHERE d.company_id = cid
    UNION ALL
    -- Filed against a contact it currently employs.
    SELECT max(a.occurred_at)
      FROM relationship r
      JOIN activity_link l ON l.contact_id = r.contact_id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND a.origin NOT IN ('system_remediation', 'system_notice')
       AND (a.kind <> 'meeting' OR (a.meeting_status IS NULL OR a.meeting_status IN ('booked','held')))
     WHERE r.company_id = cid AND r.kind = 'employment'
       AND r.ended_at IS NULL AND r.archived_at IS NULL
  ) arms
$$;

-- Calling a meeting off writes meeting_status alone, so the trigger that
-- refolds all three clocks must fire on it, and on kind, which the clause
-- reads too.
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
