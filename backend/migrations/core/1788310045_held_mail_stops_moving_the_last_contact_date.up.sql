-- "Contacted 2 days ago" must not be true because of mail the reader cannot open.
--
-- last_activity_at is a stored column on deal, organization, person and project,
-- and 43 Go read sites read it. None of them filtered audience because none of
-- them could: the value arrives from these four SQL helpers and two triggers,
-- and they counted every unarchived activity. So a message limited to its
-- participants moved a date every colleague sees, telling them a conversation
-- happened, when, and with whom — the fact of correspondence they were
-- deliberately not shown.
--
-- Fixing the SQL fixes all 43 readers at once, which is why this is a migration
-- and not a call-site sweep.
--
-- This narrows a stated policy, deliberately. The audience gate's corpus rule
-- (backend/gates/audiencereaders_test.go) holds that a reader taking
-- max(occurred_at) off an activity owes it nothing, and that stays true for
-- every OTHER marker reader in the tree. It stops being true for these four
-- columns because they are read as a sentence about a person on a page, not as
-- an internal count.

-- Bounded: CREATE OR REPLACE FUNCTION takes an exclusive lock on the function,
-- and the triggers below take one on activity, which every capture writes.
SET LOCAL lock_timeout = '3s';

-- Replacing a baseline function in a later migration, which has precedent twice
-- (1787400000, 1787450422). The bodies are the baseline's, with one clause
-- added to each join.
CREATE OR REPLACE FUNCTION last_activity_of_deal(did uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(a.occurred_at)
    FROM activity_link l
    JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
     AND a.audience = 'workspace'
   WHERE l.deal_id = did
$$;

CREATE OR REPLACE FUNCTION last_activity_of_organization(oid uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(v) FROM (
    -- Filed against the account itself.
    SELECT max(a.occurred_at) AS v
      FROM activity_link l
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
     WHERE l.organization_id = oid
    UNION ALL
    -- Filed against one of its deals.
    SELECT max(a.occurred_at)
      FROM deal d
      JOIN activity_link l ON l.deal_id = d.id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
     WHERE d.organization_id = oid
    UNION ALL
    -- Filed against a contact it currently employs.
    SELECT max(a.occurred_at)
      FROM relationship r
      JOIN activity_link l ON l.person_id = r.person_id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
     WHERE r.organization_id = oid AND r.kind = 'employment'
       AND r.ended_at IS NULL AND r.archived_at IS NULL
  ) arms
$$;

CREATE OR REPLACE FUNCTION last_activity_of_person(pid uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(a.occurred_at)
    FROM activity_link l
    JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
     AND a.audience = 'workspace'
   WHERE l.person_id = pid
$$;

-- The fourth, and the one an earlier draft of this change missed. Project
-- surfaces read and order by this value exactly as the other three do, so
-- rebuilding the project trigger below without it would have recomputed the
-- same leaking number.
CREATE OR REPLACE FUNCTION last_activity_of_project(pid uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(a.occurred_at)
    FROM activity_link l
    JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
     AND a.audience = 'workspace'
   WHERE l.project_id = pid
$$;

-- The triggers have to learn about audience too. They fire on occurred_at and
-- archived_at only, so narrowing a message changed what the helpers WOULD
-- answer while nothing asked them again — the stored value stayed on the held
-- message until some other edit happened to move it.
DROP TRIGGER activity_last_activity ON activity;
CREATE TRIGGER activity_last_activity
	AFTER UPDATE OF occurred_at, archived_at, audience ON activity
	FOR EACH ROW
	WHEN (old.occurred_at IS DISTINCT FROM new.occurred_at
	   OR old.archived_at IS DISTINCT FROM new.archived_at
	   OR old.audience IS DISTINCT FROM new.audience)
	EXECUTE FUNCTION trg_activity_last_activity();

DROP TRIGGER activity_project_last_activity ON activity;
CREATE TRIGGER activity_project_last_activity
	AFTER UPDATE OF occurred_at, archived_at, audience ON activity
	FOR EACH ROW
	WHEN (old.occurred_at IS DISTINCT FROM new.occurred_at
	   OR old.archived_at IS DISTINCT FROM new.archived_at
	   OR old.audience IS DISTINCT FROM new.audience)
	EXECUTE FUNCTION trg_activity_project_last_activity();

-- Every stored value was computed by the old helpers, so every one of them may
-- name a held message. Recomputed through the same helpers the triggers use, so
-- there is one definition of the value rather than a migration's own copy.
UPDATE deal SET last_activity_at = last_activity_of_deal(id);
UPDATE organization SET last_activity_at = last_activity_of_organization(id);
UPDATE person SET last_activity_at = last_activity_of_person(id);
UPDATE project SET last_activity_at = last_activity_of_project(id);
