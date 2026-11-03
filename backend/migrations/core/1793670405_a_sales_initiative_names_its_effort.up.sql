SET LOCAL lock_timeout = '3s';

-- A sales initiative is one commercial effort: its objective, the rules or
-- picks that say which leads and deals belong, and the baseline frozen when
-- it was launched. The rules are predicate trees in the list vocabulary of
-- the record type, evaluated by the one filter engine lists use.
CREATE TABLE sales_initiative (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    name text NOT NULL,
    description text,
    owner_id uuid NOT NULL REFERENCES app_user(id),
    objective_metric text NOT NULL,
    target_value bigint
        CONSTRAINT sales_initiative_target_value_check CHECK (target_value IS NULL OR target_value > 0)
        CONSTRAINT sales_initiative_target_value_exact CHECK (target_value BETWEEN -9007199254740991 AND 9007199254740991),
    deadline date NOT NULL,
    status text NOT NULL DEFAULT 'draft'
        CONSTRAINT sales_initiative_status_check CHECK (status IN ('draft', 'live', 'ended')),
    launch_at timestamptz,
    lead_rule jsonb,
    deal_rule jsonb,
    include_future boolean NOT NULL DEFAULT true,
    source text NOT NULL,
    captured_by text NOT NULL,
    version bigint NOT NULL DEFAULT 1
        CONSTRAINT sales_initiative_version_exact CHECK (version BETWEEN -9007199254740991 AND 9007199254740991),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,
    CONSTRAINT sales_initiative_live_has_launch CHECK (status = 'draft' OR launch_at IS NOT NULL)
);

CREATE INDEX sales_initiative_status_idx ON sales_initiative (status) WHERE archived_at IS NULL;

-- Every saved definition, with the reason it was saved, so a membership edit
-- after launch has a history a reader can follow.
CREATE TABLE sales_initiative_revision (
    initiative_id uuid NOT NULL REFERENCES sales_initiative(id) ON DELETE CASCADE,
    revision bigint NOT NULL
        CONSTRAINT sales_initiative_revision_exact CHECK (revision BETWEEN -9007199254740991 AND 9007199254740991),
    definition jsonb NOT NULL,
    reason text,
    created_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT sales_initiative_revision_pkey PRIMARY KEY (initiative_id, revision)
);

-- A record someone picked into the initiative by hand, or took out of it. A
-- rule match is evaluated live and never written here; an exclusion stands
-- over a rule match.
CREATE TABLE sales_initiative_member (
    initiative_id uuid NOT NULL REFERENCES sales_initiative(id) ON DELETE CASCADE,
    record_type text NOT NULL CONSTRAINT sales_initiative_member_type_check CHECK (record_type IN ('lead', 'deal')),
    record_id uuid NOT NULL,
    reason text NOT NULL CONSTRAINT sales_initiative_member_reason_check CHECK (reason IN ('picked', 'excluded')),
    entered_at timestamptz NOT NULL DEFAULT now(),
    captured_by text NOT NULL,
    CONSTRAINT sales_initiative_member_pkey PRIMARY KEY (initiative_id, record_type, record_id)
);

CREATE INDEX sales_initiative_member_record_idx ON sales_initiative_member (record_type, record_id);

-- The deals that belonged at launch, with the value each held then. Written
-- once by the launch and never rewritten, so a later membership edit cannot
-- move what the initiative started with.
CREATE TABLE sales_initiative_baseline (
    initiative_id uuid NOT NULL REFERENCES sales_initiative(id) ON DELETE CASCADE,
    deal_id uuid NOT NULL REFERENCES deal(id) ON DELETE CASCADE,
    amount_minor_base bigint
        CONSTRAINT sales_initiative_baseline_amount_exact CHECK (amount_minor_base BETWEEN -9007199254740991 AND 9007199254740991),
    status text NOT NULL,
    stage_id uuid,
    captured_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT sales_initiative_baseline_pkey PRIMARY KEY (initiative_id, deal_id)
);

-- The grants the compiled defaults carry, reached into the installations that
-- bootstrapped before the object existed. Managers, management and admins
-- define and launch initiatives; every other seat reads them.
UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,sales_initiative}',
        '{"create": true, "read": true, "update": true, "delete": true}'::jsonb, true)
    WHERE is_system
      AND key IN ('admin', 'management', 'manager')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'sales_initiative';

UPDATE role SET permissions = jsonb_set(
        permissions, '{objects,sales_initiative}',
        '{"create": false, "read": true, "update": false, "delete": false}'::jsonb, true)
    WHERE is_system
      AND key IN ('rep', 'read_only', 'ops')
      AND permissions ? 'objects'
      AND NOT (permissions -> 'objects') ? 'sales_initiative';

-- The member list publishes a deal's base amount as a JSON number, so the
-- column is bounded to what a JavaScript client holds exactly.
ALTER TABLE deal ADD CONSTRAINT deal_amount_minor_base_js_safe CHECK (amount_minor_base BETWEEN -9007199254740991 AND 9007199254740991) NOT VALID;

-- A retired member cannot be handed a live initiative.
CREATE TRIGGER sales_initiative_owner_id_retirement_guard BEFORE INSERT OR UPDATE ON sales_initiative
    FOR EACH ROW WHEN (NEW.owner_id IS NOT NULL AND (NEW.archived_at IS NULL))
    EXECUTE FUNCTION refuse_retired_member_work('owner_id');
