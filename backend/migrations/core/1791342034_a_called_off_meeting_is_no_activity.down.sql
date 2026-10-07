-- Reverse of the up migration: a called-off meeting counts toward
-- last_activity_at on deals, contacts and companies again, and the trigger
-- stops firing on kind and meeting_status. Stored values are refolded under
-- the restored rule.
SET LOCAL lock_timeout = '3s';

CREATE OR REPLACE FUNCTION last_activity_of_deal(did uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(a.occurred_at)
    FROM activity_link l
    JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
     AND a.audience = 'workspace'
     AND a.origin NOT IN ('system_remediation', 'system_notice')
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
     WHERE l.company_id = cid
    UNION ALL
    -- Filed against one of its deals.
    SELECT max(a.occurred_at)
      FROM deal d
      JOIN activity_link l ON l.deal_id = d.id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND a.origin NOT IN ('system_remediation', 'system_notice')
     WHERE d.company_id = cid
    UNION ALL
    -- Filed against a contact it currently employs.
    SELECT max(a.occurred_at)
      FROM relationship r
      JOIN activity_link l ON l.contact_id = r.contact_id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND a.origin NOT IN ('system_remediation', 'system_notice')
     WHERE r.company_id = cid AND r.kind = 'employment'
       AND r.ended_at IS NULL AND r.archived_at IS NULL
  ) arms
$$;

DROP TRIGGER activity_last_activity ON activity;
CREATE TRIGGER activity_last_activity
	AFTER UPDATE OF occurred_at, archived_at, audience, origin ON activity
	FOR EACH ROW
	WHEN (old.occurred_at IS DISTINCT FROM new.occurred_at
	   OR old.archived_at IS DISTINCT FROM new.archived_at
	   OR old.audience IS DISTINCT FROM new.audience
	   OR old.origin IS DISTINCT FROM new.origin)
	EXECUTE FUNCTION trg_activity_last_activity();

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
  FOR r IN
    SELECT id FROM contact
     WHERE last_activity_at IS DISTINCT FROM last_activity_of_contact(id)
     ORDER BY id
  LOOP
    PERFORM move_last_activity('contact'::regclass, r.id);
  END LOOP;
  FOR r IN
    SELECT id FROM company
     WHERE last_activity_at IS DISTINCT FROM last_activity_of_company(id)
     ORDER BY id
  LOOP
    PERFORM move_last_activity('company'::regclass, r.id);
  END LOOP;
END $$;
