-- Back to counting every unarchived activity, held or not.
--
-- The recompute at the end is what makes this a real reversal rather than a
-- schema-only one: the up migration rewrote every stored last_activity_at, so
-- leaving them alone here would keep the narrowed values under helpers that no
-- longer produce them, and the next unrelated edit to any row would move its
-- date for no reason a reader could see.

-- Bounded, with the same caveat the up migration states: lock_timeout bounds the
-- wait, not the hold, and the recompute at the bottom runs inside the same
-- transaction as the trigger swap.
SET LOCAL lock_timeout = '3s';

-- The baseline bodies, restored verbatim.
CREATE OR REPLACE FUNCTION last_activity_of_deal(did uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(a.occurred_at)
    FROM activity_link l
    JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
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
       WHERE l.organization_id = oid
    UNION ALL
    -- Filed against one of its deals.
    SELECT max(a.occurred_at)
      FROM deal d
      JOIN activity_link l ON l.deal_id = d.id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       WHERE d.organization_id = oid
    UNION ALL
    -- Filed against a contact it currently employs.
    SELECT max(a.occurred_at)
      FROM relationship r
      JOIN activity_link l ON l.person_id = r.person_id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
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
   WHERE l.person_id = pid
$$;

CREATE OR REPLACE FUNCTION last_activity_of_project(pid uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(a.occurred_at)
    FROM activity_link l
    JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
   WHERE l.project_id = pid
$$;

-- The triggers stop watching audience: with the helpers ignoring it again, a
-- narrowing changes nothing they would answer, so waking them for it would only
-- burn a recompute.
DROP TRIGGER activity_last_activity ON activity;
CREATE TRIGGER activity_last_activity
	AFTER UPDATE OF occurred_at, archived_at ON activity
	FOR EACH ROW
	WHEN (old.occurred_at IS DISTINCT FROM new.occurred_at
	   OR old.archived_at IS DISTINCT FROM new.archived_at)
	EXECUTE FUNCTION trg_activity_last_activity();

DROP TRIGGER activity_project_last_activity ON activity;
CREATE TRIGGER activity_project_last_activity
	AFTER UPDATE OF occurred_at, archived_at ON activity
	FOR EACH ROW
	WHEN (old.occurred_at IS DISTINCT FROM new.occurred_at
	   OR old.archived_at IS DISTINCT FROM new.archived_at)
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
