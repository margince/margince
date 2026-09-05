-- Reverse of the up migration: the notice origin stops being a member, and the
-- four helpers go back to excluding remediation alone.
--
-- Any activity already written as system_notice would fail the narrowed CHECK,
-- so they are moved to system_remediation rather than deleted: they are real
-- outbound messages with deliveries hanging off them, and the nearest surviving
-- origin that also stays out of the recency helpers keeps both facts true.
SET LOCAL lock_timeout = '3s';

UPDATE activity SET origin = 'system_remediation' WHERE origin = 'system_notice';

ALTER TABLE activity DROP CONSTRAINT activity_origin_check;

ALTER TABLE activity
    ADD CONSTRAINT activity_origin_check
        CHECK (origin IN ('human', 'agent', 'system_remediation')) NOT VALID;

ALTER TABLE activity VALIDATE CONSTRAINT activity_origin_check;

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
