-- A Live List's members join and leave on their own, so the only way to say
-- who did is to look regularly and compare. The evaluator keeps the last set
-- it saw per list, records each difference as an entered or left event, and
-- checkpoints each list so a pass resumes with the lists checked longest ago.
-- A reader also keeps when they last opened a list, so the list can say what
-- changed since.
SET LOCAL lock_timeout = '5s';

-- Observed changes are stamped with the definition version they were seen
-- under: the first check after the filter changed reports the difference the
-- new filter made, and says so.
ALTER TABLE list_member_event
    ADD COLUMN definition_version bigint,
    DROP CONSTRAINT list_member_event_action_check,
    ADD CONSTRAINT list_member_event_action_check CHECK (action IN ('added', 'removed', 'entered', 'left')),
    DROP CONSTRAINT list_member_event_reason_check,
    ADD CONSTRAINT list_member_event_reason_check CHECK (reason IN ('chosen', 'bulk', 'record_archived', 'record_restored', 'evaluated', 'filter_changed'));

-- The members a Live List had at its last check. Derived, and rebuilt by the
-- next check; it carries no record content, only which records matched.
CREATE TABLE list_live_member (
    list_id uuid NOT NULL,
    entity_type text NOT NULL,
    entity_id uuid NOT NULL,
    member_since timestamptz NOT NULL,
    definition_version bigint NOT NULL,

    CONSTRAINT list_live_member_pkey PRIMARY KEY (list_id, entity_id),
    CONSTRAINT list_live_member_list_id_fkey FOREIGN KEY (list_id) REFERENCES list(id) ON DELETE CASCADE,
    CONSTRAINT list_live_member_entity_type_check CHECK (entity_type IN ('contact', 'company', 'deal', 'lead', 'project'))
);

CREATE INDEX idx_list_live_member_entity ON list_live_member USING btree (entity_type, entity_id);

-- The last check of each Live List: when, under which definition version,
-- how many records matched, and how it ended. complete wrote the difference;
-- too_large matched more records than one check may hold and wrote nothing;
-- invalid could not evaluate the filter. snapshot_version is the version the
-- held members were last taken under, null until a check first completes.
CREATE TABLE list_evaluation (
    list_id uuid NOT NULL,
    definition_version bigint NOT NULL,
    evaluated_at timestamptz NOT NULL,
    member_count integer NOT NULL,
    outcome text NOT NULL,
    snapshot_version bigint,

    CONSTRAINT list_evaluation_pkey PRIMARY KEY (list_id),
    CONSTRAINT list_evaluation_list_id_fkey FOREIGN KEY (list_id) REFERENCES list(id) ON DELETE CASCADE,
    CONSTRAINT list_evaluation_outcome_check CHECK (outcome IN ('complete', 'too_large', 'invalid'))
);

CREATE INDEX idx_list_evaluation_due ON list_evaluation USING btree (evaluated_at);

-- When a user last opened a list, and the visit before that: "since your
-- last visit" counts from the visit before the one still in progress, so a
-- page read after its own visit is recorded still shows what was new. Their
-- own view state, read by nobody else.
CREATE TABLE list_visit (
    user_id uuid NOT NULL,
    list_id uuid NOT NULL,
    visited_at timestamptz NOT NULL,
    previous_visited_at timestamptz,

    CONSTRAINT list_visit_pkey PRIMARY KEY (user_id, list_id),
    CONSTRAINT list_visit_user_id_fkey FOREIGN KEY (user_id) REFERENCES app_user(id) ON DELETE CASCADE,
    CONSTRAINT list_visit_list_id_fkey FOREIGN KEY (list_id) REFERENCES list(id) ON DELETE CASCADE
);

CREATE INDEX idx_list_visit_list ON list_visit USING btree (list_id);
