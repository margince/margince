SET LOCAL lock_timeout = '3s';
CREATE TABLE reporting_framework (
    id uuid NOT NULL UNIQUE DEFAULT uuidv7(),
    singleton boolean PRIMARY KEY DEFAULT true CHECK (singleton),
    revision bigint NOT NULL,
    version bigint NOT NULL
);
CREATE TABLE reporting_framework_revision (
    revision bigint PRIMARY KEY,
    definition jsonb NOT NULL,
    effective_at timestamptz NOT NULL,
    created_by uuid NOT NULL REFERENCES app_user(id)
);
CREATE TABLE report_definition (
    id uuid PRIMARY KEY,
    owner_id uuid NOT NULL REFERENCES app_user(id),
    name text NOT NULL,
    audience text NOT NULL CHECK (audience IN ('private','team','workspace')),
    audience_team_id uuid REFERENCES team(id),
    revision bigint NOT NULL,
    version bigint NOT NULL,
    created_at timestamptz NOT NULL,
    archived_at timestamptz,
    CHECK ((audience = 'team') = (audience_team_id IS NOT NULL))
);
CREATE INDEX report_definition_library ON report_definition(owner_id, audience, name, id) WHERE archived_at IS NULL;
CREATE TABLE report_definition_revision (
    report_id uuid NOT NULL REFERENCES report_definition(id),
    revision bigint NOT NULL,
    selection jsonb NOT NULL,
    framework_revision bigint NOT NULL,
    created_at timestamptz NOT NULL,
    created_by uuid NOT NULL REFERENCES app_user(id),
    PRIMARY KEY (report_id, revision)
);
CREATE TABLE sales_target (
    id uuid PRIMARY KEY,
    metric text NOT NULL,
    scope_kind text NOT NULL CHECK (scope_kind IN ('owner','team','workspace')),
    scope_id uuid,
    pipeline_id uuid REFERENCES pipeline(id),
    period_kind text NOT NULL CHECK (period_kind IN ('month','fiscal_quarter')),
    period_start timestamptz NOT NULL,
    period_end timestamptz NOT NULL,
    unit text NOT NULL,
    revision bigint NOT NULL,
    version bigint NOT NULL,
    created_at timestamptz NOT NULL,
    CHECK (period_end > period_start),
    CHECK ((scope_kind = 'workspace') = (scope_id IS NULL))
);
CREATE UNIQUE INDEX sales_target_identity ON sales_target(metric, scope_kind,
    COALESCE(scope_id::text, 'workspace'), COALESCE(pipeline_id::text, 'all'), period_kind, period_start);
CREATE TABLE sales_target_revision (
    target_id uuid NOT NULL REFERENCES sales_target(id),
    revision bigint NOT NULL,
    definition jsonb NOT NULL,
    effective_at timestamptz NOT NULL,
    created_by uuid NOT NULL REFERENCES app_user(id),
    PRIMARY KEY (target_id, revision)
);
CREATE TABLE report_schedule (
    id uuid PRIMARY KEY,
    report_id uuid NOT NULL,
    report_revision bigint NOT NULL,
    owner_id uuid NOT NULL REFERENCES app_user(id),
    definition jsonb NOT NULL,
    timezone text NOT NULL,
    enabled boolean NOT NULL,
    version bigint NOT NULL,
    next_due_at timestamptz NOT NULL,
    last_status text,
    FOREIGN KEY (report_id, report_revision) REFERENCES report_definition_revision(report_id, revision)
);
CREATE INDEX report_schedule_due ON report_schedule(next_due_at, id) WHERE enabled;
CREATE TABLE report_execution (
    id uuid PRIMARY KEY,
    report_id uuid NOT NULL,
    report_revision bigint NOT NULL,
    schedule_id uuid REFERENCES report_schedule(id),
    schedule_version bigint,
    owner_id uuid NOT NULL REFERENCES app_user(id),
    intended_due_at timestamptz NOT NULL,
    interval_start timestamptz NOT NULL,
    interval_end timestamptz NOT NULL,
    status text NOT NULL CHECK (status IN ('pending','running','succeeded','partial','failed','suspended','skipped')),
    attempt bigint NOT NULL DEFAULT 0,
    fence bigint NOT NULL DEFAULT 0,
    lease_until timestamptz,
    retry_at timestamptz,
    execution_key text NOT NULL UNIQUE,
    request_hash text NOT NULL,
    reason text,
    edition_id uuid,
    FOREIGN KEY (report_id, report_revision) REFERENCES report_definition_revision(report_id, revision)
);
CREATE INDEX report_execution_pending ON report_execution(status, retry_at, intended_due_at, id);
CREATE TABLE report_edition (
    id uuid PRIMARY KEY,
    execution_id uuid NOT NULL UNIQUE REFERENCES report_execution(id),
    report_id uuid NOT NULL REFERENCES report_definition(id),
    report_revision bigint NOT NULL,
    publication_audience text NOT NULL,
    publication_team_id uuid,
    owner_id uuid NOT NULL,
    captured_at timestamptz NOT NULL,
    intended_due_at timestamptz NOT NULL,
    manifest jsonb NOT NULL,
    redacted_at timestamptz
);
CREATE INDEX report_edition_archive ON report_edition(report_id, intended_due_at DESC, id DESC);
CREATE TABLE report_edition_contribution (
    edition_id uuid NOT NULL REFERENCES report_edition(id) ON DELETE CASCADE,
    metric text NOT NULL,
    context_id text NOT NULL,
    contribution_key text NOT NULL,
    group_key text NOT NULL,
    source_type text NOT NULL,
    source_id uuid NOT NULL,
    owner_id uuid,
    fact jsonb NOT NULL,
    PRIMARY KEY (edition_id, metric, context_id, contribution_key)
);
CREATE INDEX report_contribution_source ON report_edition_contribution(source_type, source_id);

UPDATE role SET permissions=jsonb_set(permissions, '{objects,report_definition}', '{"create": true, "read": true, "update": true, "delete": true}'::jsonb) WHERE is_system AND key='admin' AND NOT permissions->'objects' ? 'report_definition';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,sales_target}', '{"create": true, "read": true, "update": true, "delete": false}'::jsonb) WHERE is_system AND key='admin' AND NOT permissions->'objects' ? 'sales_target';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,reporting_framework}', '{"create": false, "read": true, "update": true, "delete": false}'::jsonb) WHERE is_system AND key='admin' AND NOT permissions->'objects' ? 'reporting_framework';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,report_schedule}', '{"create": true, "read": true, "update": true, "delete": true}'::jsonb) WHERE is_system AND key='admin' AND NOT permissions->'objects' ? 'report_schedule';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,report_edition}', '{"create": true, "read": true, "update": true, "delete": false}'::jsonb) WHERE is_system AND key='admin' AND NOT permissions->'objects' ? 'report_edition';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,reporting_credit}', '{"create": false, "read": true, "update": false, "delete": false}'::jsonb) WHERE is_system AND key='admin' AND NOT permissions->'objects' ? 'reporting_credit';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,report_definition}', '{"create": true, "read": true, "update": true, "delete": true}'::jsonb) WHERE is_system AND key='management' AND NOT permissions->'objects' ? 'report_definition';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,sales_target}', '{"create": true, "read": true, "update": true, "delete": false}'::jsonb) WHERE is_system AND key='management' AND NOT permissions->'objects' ? 'sales_target';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,reporting_framework}', '{"create": false, "read": true, "update": false, "delete": false}'::jsonb) WHERE is_system AND key='management' AND NOT permissions->'objects' ? 'reporting_framework';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,report_schedule}', '{"create": true, "read": true, "update": true, "delete": true}'::jsonb) WHERE is_system AND key='management' AND NOT permissions->'objects' ? 'report_schedule';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,report_edition}', '{"create": true, "read": true, "update": true, "delete": false}'::jsonb) WHERE is_system AND key='management' AND NOT permissions->'objects' ? 'report_edition';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,reporting_credit}', '{"create": false, "read": true, "update": false, "delete": false}'::jsonb) WHERE is_system AND key='management' AND NOT permissions->'objects' ? 'reporting_credit';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,report_definition}', '{"create": true, "read": true, "update": true, "delete": true}'::jsonb) WHERE is_system AND key='manager' AND NOT permissions->'objects' ? 'report_definition';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,sales_target}', '{"create": true, "read": true, "update": true, "delete": false}'::jsonb) WHERE is_system AND key='manager' AND NOT permissions->'objects' ? 'sales_target';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,reporting_framework}', '{"create": false, "read": true, "update": false, "delete": false}'::jsonb) WHERE is_system AND key='manager' AND NOT permissions->'objects' ? 'reporting_framework';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,report_schedule}', '{"create": true, "read": true, "update": true, "delete": true}'::jsonb) WHERE is_system AND key='manager' AND NOT permissions->'objects' ? 'report_schedule';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,report_edition}', '{"create": true, "read": true, "update": true, "delete": false}'::jsonb) WHERE is_system AND key='manager' AND NOT permissions->'objects' ? 'report_edition';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,reporting_credit}', '{"create": false, "read": true, "update": false, "delete": false}'::jsonb) WHERE is_system AND key='manager' AND NOT permissions->'objects' ? 'reporting_credit';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,report_definition}', '{"create": true, "read": true, "update": true, "delete": true}'::jsonb) WHERE is_system AND key='rep' AND NOT permissions->'objects' ? 'report_definition';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,sales_target}', '{"create": false, "read": true, "update": false, "delete": false}'::jsonb) WHERE is_system AND key='rep' AND NOT permissions->'objects' ? 'sales_target';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,reporting_framework}', '{"create": false, "read": true, "update": false, "delete": false}'::jsonb) WHERE is_system AND key='rep' AND NOT permissions->'objects' ? 'reporting_framework';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,report_schedule}', '{"create": true, "read": true, "update": true, "delete": true}'::jsonb) WHERE is_system AND key='rep' AND NOT permissions->'objects' ? 'report_schedule';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,report_edition}', '{"create": true, "read": true, "update": true, "delete": false}'::jsonb) WHERE is_system AND key='rep' AND NOT permissions->'objects' ? 'report_edition';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,reporting_credit}', '{"create": false, "read": true, "update": false, "delete": false}'::jsonb) WHERE is_system AND key='rep' AND NOT permissions->'objects' ? 'reporting_credit';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,report_definition}', '{"create": false, "read": true, "update": false, "delete": false}'::jsonb) WHERE is_system AND key='read_only' AND NOT permissions->'objects' ? 'report_definition';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,sales_target}', '{"create": false, "read": true, "update": false, "delete": false}'::jsonb) WHERE is_system AND key='read_only' AND NOT permissions->'objects' ? 'sales_target';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,reporting_framework}', '{"create": false, "read": true, "update": false, "delete": false}'::jsonb) WHERE is_system AND key='read_only' AND NOT permissions->'objects' ? 'reporting_framework';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,report_schedule}', '{"create": false, "read": true, "update": false, "delete": false}'::jsonb) WHERE is_system AND key='read_only' AND NOT permissions->'objects' ? 'report_schedule';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,report_edition}', '{"create": false, "read": true, "update": false, "delete": false}'::jsonb) WHERE is_system AND key='read_only' AND NOT permissions->'objects' ? 'report_edition';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,reporting_credit}', '{"create": false, "read": true, "update": false, "delete": false}'::jsonb) WHERE is_system AND key='read_only' AND NOT permissions->'objects' ? 'reporting_credit';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,report_definition}', '{"create": false, "read": true, "update": false, "delete": false}'::jsonb) WHERE is_system AND key='ops' AND NOT permissions->'objects' ? 'report_definition';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,sales_target}', '{"create": false, "read": true, "update": false, "delete": false}'::jsonb) WHERE is_system AND key='ops' AND NOT permissions->'objects' ? 'sales_target';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,reporting_framework}', '{"create": false, "read": true, "update": false, "delete": false}'::jsonb) WHERE is_system AND key='ops' AND NOT permissions->'objects' ? 'reporting_framework';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,report_schedule}', '{"create": false, "read": true, "update": false, "delete": false}'::jsonb) WHERE is_system AND key='ops' AND NOT permissions->'objects' ? 'report_schedule';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,report_edition}', '{"create": false, "read": true, "update": false, "delete": false}'::jsonb) WHERE is_system AND key='ops' AND NOT permissions->'objects' ? 'report_edition';

UPDATE role SET permissions=jsonb_set(permissions, '{objects,reporting_credit}', '{"create": false, "read": true, "update": false, "delete": false}'::jsonb) WHERE is_system AND key='ops' AND NOT permissions->'objects' ? 'reporting_credit';

ALTER TABLE deal_stage_history ADD COLUMN owner_id_at_change uuid;
ALTER TABLE deal_stage_history ADD COLUMN pipeline_id_at_change uuid;
ALTER TABLE activity_meeting_history ADD COLUMN host_id_at_change uuid;
ALTER TABLE sdr_handoff_event ADD COLUMN deal_id_at_change uuid;
ALTER TABLE sdr_handoff_event ADD COLUMN submitter_id_at_change uuid;

ALTER TABLE deal_stage_history ADD COLUMN base_minor_at_change bigint;
ALTER TABLE deal_stage_history ADD COLUMN base_currency_at_change text;
ALTER TABLE deal_stage_history ADD COLUMN fx_rate_at_change numeric;
ALTER TABLE deal_stage_history ADD COLUMN fx_date_at_change date;
ALTER TABLE deal_stage_history ADD COLUMN valuation_provenance text;
ALTER TABLE activity_meeting_history ADD COLUMN customer_eligible_at_change boolean;
CREATE INDEX reporting_stage_events ON deal_stage_history(deal_id, changed_at, id);
CREATE INDEX reporting_primary_credit ON sdr_handoff_event(deal_id_at_change, occurred_at, id) WHERE to_status='accepted';
