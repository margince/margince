SET LOCAL lock_timeout = '3s';

-- Responsibility is not access. A record assignment says WHO is accountable for
-- a company, deal or project; it never widens who may see it. Visibility stays
-- with ownership and record_grant, which identity owns. That separation is the
-- whole reason this table exists instead of another grant kind: a sales
-- engineer can be the named technical contact on a deal they already work,
-- without that naming handing anyone a record they could not otherwise open.

-- The administered vocabulary of responsibilities. Same catalog shape as
-- deal_acquisition_source and lead_source: stable key, editable label, retire
-- rather than delete, so an assignment made a year ago still renders.
CREATE TABLE record_role (
    id             uuid PRIMARY KEY DEFAULT uuidv7(),
    key            text NOT NULL UNIQUE,
    label          text NOT NULL,
    -- Which record kinds this role may be held on, and whether a user, a team
    -- or either may hold it. Stored as arrays because the sets are tiny and
    -- read on every assignment write; a join table would buy nothing.
    record_types   text[] NOT NULL,
    assignee_kinds text[] NOT NULL,
    sort_order     integer NOT NULL DEFAULT 0,
    active         boolean NOT NULL DEFAULT true,
    -- Seeded with the installation. Every field stays editable; `system` only
    -- means the row is part of the shipped vocabulary.
    system         boolean NOT NULL DEFAULT false,
    version        bigint NOT NULL DEFAULT 1,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT record_role_key_shape
        CHECK (key = lower(key) AND length(btrim(key)) > 0),
    CONSTRAINT record_role_label_present
        CHECK (length(btrim(label)) > 0),
    -- A role applicable to nothing is unusable, and an empty array would read
    -- as "applies to all" to a careless reader. Require at least one of each.
    CONSTRAINT record_role_record_types_bounded
        CHECK (cardinality(record_types) > 0
               AND record_types <@ ARRAY['company','deal','project']::text[]),
    CONSTRAINT record_role_assignee_kinds_bounded
        CHECK (cardinality(assignee_kinds) > 0
               AND assignee_kinds <@ ARRAY['user','team']::text[])
);

CREATE TRIGGER trg_record_role_updated
    BEFORE UPDATE ON record_role
    FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();

INSERT INTO record_role (key, label, record_types, assignee_kinds, sort_order, system) VALUES
    ('account_manager',    'Account manager',    ARRAY['company','deal','project'], ARRAY['user'],        10, true),
    ('sales_engineer',     'Sales engineer',     ARRAY['deal'],                     ARRAY['user'],        20, true),
    ('delivery_lead',      'Delivery lead',      ARRAY['project'],                  ARRAY['user'],        30, true),
    ('project_manager',    'Project manager',    ARRAY['project'],                  ARRAY['user'],        40, true),
    ('technical_contact',  'Technical contact',  ARRAY['company','deal','project'], ARRAY['user','team'], 50, true),
    ('executive_sponsor',  'Executive sponsor',  ARRAY['company','deal'],           ARRAY['user'],        60, true),
    ('support_team',       'Support team',       ARRAY['company','project'],        ARRAY['team'],        70, true);


-- One responsibility held on one record.
--
-- The parent and the assignee are both typed FK arms rather than a
-- (type, uuid) pair: a polymorphic id cannot be checked by the database, and an
-- assignment pointing at a deleted deal is exactly the row that later renders
-- as a blank name nobody can explain. num_nonnulls collapses the arms back to
-- the single logical parent the API exposes.
CREATE TABLE record_assignment (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),

    company_id  uuid REFERENCES company(id) ON DELETE CASCADE,
    deal_id     uuid REFERENCES deal(id)    ON DELETE CASCADE,
    project_id  uuid REFERENCES project(id) ON DELETE CASCADE,

    -- RESTRICT, not CASCADE: who was responsible is history worth keeping, and
    -- a user row is deactivated rather than deleted in the ordinary path.
    user_id     uuid REFERENCES app_user(id) ON DELETE RESTRICT,
    team_id     uuid REFERENCES team(id)     ON DELETE RESTRICT,

    role_id     uuid NOT NULL REFERENCES record_role(id) ON DELETE RESTRICT,

    source      text,
    -- text, and deliberately NOT a key into app_user: captured_by holds a
    -- PREFIXED principal ("human:<id>" / "agent:<id>"), and an agent acting
    -- under a passport is not a row in app_user at all. A foreign key here
    -- would refuse the write rather than record who made it.
    captured_by text,

    version     bigint NOT NULL DEFAULT 1,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,

    CONSTRAINT record_assignment_one_parent
        CHECK (num_nonnulls(company_id, deal_id, project_id) = 1),
    CONSTRAINT record_assignment_one_assignee
        CHECK (num_nonnulls(user_id, team_id) = 1)
);

CREATE TRIGGER trg_record_assignment_updated
    BEFORE UPDATE ON record_assignment
    FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();

-- Six partial unique indexes, one per parent arm x assignee arm. SQL NULL is
-- never equal to itself, so a single unique index over all five columns would
-- admit unlimited duplicates; each index therefore names only the columns that
-- are non-null in its own case. Live rows only, so archiving frees the slot for
-- a successor without erasing who held it.
CREATE UNIQUE INDEX uq_record_assignment_company_user
    ON record_assignment (company_id, role_id, user_id)
    WHERE company_id IS NOT NULL AND user_id IS NOT NULL AND archived_at IS NULL;
CREATE UNIQUE INDEX uq_record_assignment_company_team
    ON record_assignment (company_id, role_id, team_id)
    WHERE company_id IS NOT NULL AND team_id IS NOT NULL AND archived_at IS NULL;
CREATE UNIQUE INDEX uq_record_assignment_deal_user
    ON record_assignment (deal_id, role_id, user_id)
    WHERE deal_id IS NOT NULL AND user_id IS NOT NULL AND archived_at IS NULL;
CREATE UNIQUE INDEX uq_record_assignment_deal_team
    ON record_assignment (deal_id, role_id, team_id)
    WHERE deal_id IS NOT NULL AND team_id IS NOT NULL AND archived_at IS NULL;
CREATE UNIQUE INDEX uq_record_assignment_project_user
    ON record_assignment (project_id, role_id, user_id)
    WHERE project_id IS NOT NULL AND user_id IS NOT NULL AND archived_at IS NULL;
CREATE UNIQUE INDEX uq_record_assignment_project_team
    ON record_assignment (project_id, role_id, team_id)
    WHERE project_id IS NOT NULL AND team_id IS NOT NULL AND archived_at IS NULL;

-- Parent lookups serve the record pages; assignee lookups serve "my
-- assignments" and the three list filters.
CREATE INDEX idx_record_assignment_company  ON record_assignment (company_id) WHERE company_id IS NOT NULL AND archived_at IS NULL;
CREATE INDEX idx_record_assignment_deal     ON record_assignment (deal_id)    WHERE deal_id    IS NOT NULL AND archived_at IS NULL;
CREATE INDEX idx_record_assignment_project  ON record_assignment (project_id) WHERE project_id IS NOT NULL AND archived_at IS NULL;
CREATE INDEX idx_record_assignment_user     ON record_assignment (user_id)    WHERE user_id    IS NOT NULL AND archived_at IS NULL;
CREATE INDEX idx_record_assignment_team     ON record_assignment (team_id)    WHERE team_id    IS NOT NULL AND archived_at IS NULL;
