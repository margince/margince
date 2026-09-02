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
-- and the trigger swap below takes one on activity, which every capture writes.
--
-- lock_timeout bounds only the WAIT to acquire those locks, never how long this
-- transaction then holds them, and the migration runner keeps each file in one
-- transaction — so the activity lock is held until the recompute at the bottom
-- finishes. Two things keep that window short rather than proportional to the
-- installation: the recompute touches only rows whose value actually moves (an
-- installation with no held mail writes nothing at all), and every helper it
-- calls is index-served through activity_link. It is still a write pause on
-- capture, and this is where a deploy runbook would read that it exists.
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

-- Recomputed through move_last_activity, not a raw UPDATE.
--
-- That is not a style preference. set_updated_at_bump_version() fires on all
-- four tables and suppresses itself only while margince.last_activity_move is
-- on, which move_last_activity sets and a bare UPDATE does not. Writing the
-- column directly would stamp updated_at and bump version on every deal,
-- organization, person and project in the installation — invalidating every
-- outstanding If-Match a client holds and telling every "what changed
-- recently" reader that the whole database was edited at deploy time.
--
-- Only rows whose derived value actually moves are touched, so an installation
-- with no held mail writes nothing at all.
DO $$
DECLARE
  r record;
BEGIN
  FOR r IN
    SELECT id FROM deal
     WHERE last_activity_at IS DISTINCT FROM last_activity_of_deal(id)
  LOOP
    PERFORM move_last_activity('deal'::regclass, r.id);
  END LOOP;
  FOR r IN
    SELECT id FROM organization
     WHERE last_activity_at IS DISTINCT FROM last_activity_of_organization(id)
  LOOP
    PERFORM move_last_activity('organization'::regclass, r.id);
  END LOOP;
  FOR r IN
    SELECT id FROM person
     WHERE last_activity_at IS DISTINCT FROM last_activity_of_person(id)
  LOOP
    PERFORM move_last_activity('person'::regclass, r.id);
  END LOOP;
  FOR r IN
    SELECT id FROM project
     WHERE last_activity_at IS DISTINCT FROM last_activity_of_project(id)
  LOOP
    PERFORM move_last_activity('project'::regclass, r.id);
  END LOOP;
END $$;
