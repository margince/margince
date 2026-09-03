-- Work the system files about a record is not evidence the buyer engaged.
--
-- Forecast assurance will file review tasks against a deal: "confirm the close
-- date", "the offer says 120k and the deal says 480k". Those are activities,
-- and every recency reading in the product folds the newest activity into
-- last_activity_at through the four helpers below. Left alone, the system
-- asking a question about a silent deal would make that deal look freshly
-- touched, and the very rule that noticed the silence would stop firing. The
-- engine would switch itself off, one deal at a time, and nothing would fail.
--
-- origin says who caused the row to exist. Every existing row is human because
-- a human caused every one of them: there is no other writer yet.
--
-- These four helpers already carry `a.audience = 'workspace'`, added because a
-- message limited to its participants was moving a date every colleague could
-- see. That clause is repeated verbatim here — a CREATE OR REPLACE writes the
-- whole body, so a copy taken from the baseline instead of from the live
-- definition would silently delete it and re-open that hole.
--
-- Bounded, because these are called by triggers on every activity link write:
-- an open transaction holding a conflicting lock would otherwise stall every
-- capture for as long as this migration is willing to queue.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity
    ADD COLUMN origin text NOT NULL DEFAULT 'human';

-- NOT VALID first, then VALIDATE: a validated CHECK scans the whole table under
-- ACCESS EXCLUSIVE, and lock_timeout bounds only how long we WAIT for the lock,
-- never how long we hold it once scanning. On an installation whose activity
-- table holds years of captured mail that is every capture blocked for the
-- length of the scan. NOT VALID takes the lock briefly, VALIDATE re-reads under
-- a weaker one, and the constraint ends up enforced either way.
ALTER TABLE activity
    ADD CONSTRAINT activity_origin_check
        CHECK (origin IN ('human', 'agent', 'system_remediation')) NOT VALID;

ALTER TABLE activity VALIDATE CONSTRAINT activity_origin_check;

-- The default exists so the ALTER does not rewrite the table, not so a caller
-- can stay silent: LogActivityInput.Origin states it on every write.

CREATE OR REPLACE FUNCTION last_activity_of_deal(did uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(a.occurred_at)
    FROM activity_link l
    JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
     AND a.audience = 'workspace'
     AND a.origin <> 'system_remediation'
   WHERE l.deal_id = did
$$;

CREATE OR REPLACE FUNCTION last_activity_of_person(pid uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(a.occurred_at)
    FROM activity_link l
    JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
     AND a.audience = 'workspace'
     AND a.origin <> 'system_remediation'
   WHERE l.person_id = pid
$$;

-- Three arms, three clauses: an account is as engaged as the newest real touch
-- on itself, on its deals, or on someone it employs. Missing one arm would let
-- a remediation task filed against a deal refresh its account.
CREATE OR REPLACE FUNCTION last_activity_of_organization(oid uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(v) FROM (
    -- Filed against the account itself.
    SELECT max(a.occurred_at) AS v
      FROM activity_link l
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND a.origin <> 'system_remediation'
     WHERE l.organization_id = oid
    UNION ALL
    -- Filed against one of its deals.
    SELECT max(a.occurred_at)
      FROM deal d
      JOIN activity_link l ON l.deal_id = d.id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND a.origin <> 'system_remediation'
     WHERE d.organization_id = oid
    UNION ALL
    -- Filed against a contact it currently employs.
    SELECT max(a.occurred_at)
      FROM relationship r
      JOIN activity_link l ON l.person_id = r.person_id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND a.origin <> 'system_remediation'
     WHERE r.organization_id = oid AND r.kind = 'employment'
       AND r.ended_at IS NULL AND r.archived_at IS NULL
  ) arms
$$;

CREATE OR REPLACE FUNCTION last_activity_of_project(pid uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(a.occurred_at)
    FROM activity_link l
    JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
     AND a.audience = 'workspace'
     AND a.origin <> 'system_remediation'
   WHERE l.project_id = pid
$$;

-- The triggers have to learn about origin too, for the same reason they had to
-- learn about audience: they fire on occurred_at, archived_at and audience, so
-- relabelling an activity's origin would change what the helpers WOULD answer
-- while nothing asked them again, leaving the stored clock on its old value
-- until some unrelated edit happened to move it.
DROP TRIGGER activity_last_activity ON activity;
CREATE TRIGGER activity_last_activity
	AFTER UPDATE OF occurred_at, archived_at, audience, origin ON activity
	FOR EACH ROW
	WHEN (old.occurred_at IS DISTINCT FROM new.occurred_at
	   OR old.archived_at IS DISTINCT FROM new.archived_at
	   OR old.audience IS DISTINCT FROM new.audience
	   OR old.origin IS DISTINCT FROM new.origin)
	EXECUTE FUNCTION trg_activity_last_activity();

DROP TRIGGER activity_project_last_activity ON activity;
CREATE TRIGGER activity_project_last_activity
	AFTER UPDATE OF occurred_at, archived_at, audience, origin ON activity
	FOR EACH ROW
	WHEN (old.occurred_at IS DISTINCT FROM new.occurred_at
	   OR old.archived_at IS DISTINCT FROM new.archived_at
	   OR old.audience IS DISTINCT FROM new.audience
	   OR old.origin IS DISTINCT FROM new.origin)
	EXECUTE FUNCTION trg_activity_project_last_activity();
