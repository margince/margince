-- A notice the installation owes somebody is not evidence they engaged.
--
-- The confirm-details mail and the double-opt-in link are activities: they are
-- outbound messages and belong on the timeline, because a person asking "why
-- did you write to me" deserves to find the answer there, and privacy's
-- erasure and the subject-access export both reach comms_outbound THROUGH the
-- activity row.
--
-- But the installation writing to somebody about its own obligations says
-- nothing about whether that person is engaged, and every recency reading in
-- the product folds the newest activity into last_activity_at through the four
-- helpers below. Left alone, mailing a dormant contact to ask them to check
-- their details would make them look freshly active — and the very silence
-- that would prompt someone to reach out would be erased by the asking.
--
-- Exactly the argument 1788386600 made for system_remediation. The four helper
-- bodies here are that migration's, copied whole and with only the origin
-- clause widened: a CREATE OR REPLACE writes the entire body, so a copy taken
-- from anywhere but the live definition would silently drop a clause. Both the
-- audience filter and the remediation exclusion are therefore repeated verbatim
-- rather than restated.
--
-- Bounded, because these are called by triggers on every activity link write.
SET LOCAL lock_timeout = '3s';

-- NOT VALID first, then VALIDATE, for the reason the earlier migration states:
-- a validated CHECK scans the whole table under ACCESS EXCLUSIVE, which on an
-- installation holding years of captured mail blocks every capture for the
-- length of the scan.
ALTER TABLE activity DROP CONSTRAINT activity_origin_check;

ALTER TABLE activity
    ADD CONSTRAINT activity_origin_check
        CHECK (origin IN ('human', 'agent', 'system_remediation', 'system_notice')) NOT VALID;

ALTER TABLE activity VALIDATE CONSTRAINT activity_origin_check;

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

CREATE OR REPLACE FUNCTION last_activity_of_person(pid uuid) RETURNS timestamptz
    LANGUAGE sql STABLE
    AS $$
  SELECT max(a.occurred_at)
    FROM activity_link l
    JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
     AND a.audience = 'workspace'
     AND a.origin NOT IN ('system_remediation', 'system_notice')
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
       AND a.origin NOT IN ('system_remediation', 'system_notice')
     WHERE l.organization_id = oid
    UNION ALL
    -- Filed against one of its deals.
    SELECT max(a.occurred_at)
      FROM deal d
      JOIN activity_link l ON l.deal_id = d.id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND a.origin NOT IN ('system_remediation', 'system_notice')
     WHERE d.organization_id = oid
    UNION ALL
    -- Filed against a contact it currently employs.
    SELECT max(a.occurred_at)
      FROM relationship r
      JOIN activity_link l ON l.person_id = r.person_id
      JOIN activity a ON a.id = l.activity_id AND a.archived_at IS NULL
       AND a.audience = 'workspace'
       AND a.origin NOT IN ('system_remediation', 'system_notice')
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
     AND a.origin NOT IN ('system_remediation', 'system_notice')
   WHERE l.project_id = pid
$$;
