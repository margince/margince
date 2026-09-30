-- A list becomes a shared working set: it says what it is for, who looks after
-- it and who may find it, and it keeps the history a reader needs to trust it.
--
-- sharing decides who may DISCOVER a list: its steward and owner alone
-- (private), the owner's team or the team the list names (team), or every seat
-- (workspace). It never decides who may see a member record; every member read
-- stays row-scoped to the reader.
SET LOCAL lock_timeout = '5s';

ALTER TABLE list
    ADD COLUMN purpose text,
    ADD COLUMN steward_id uuid,
    ADD COLUMN sharing text DEFAULT 'team'::text NOT NULL,
    ADD CONSTRAINT list_sharing_check CHECK (sharing IN ('private', 'team', 'workspace')),
    ADD CONSTRAINT list_steward_id_fkey FOREIGN KEY (steward_id) REFERENCES app_user(id) ON DELETE SET NULL;

-- The creator looked after every list that exists today.
UPDATE list SET steward_id = owner_id WHERE steward_id IS NULL AND owner_id IS NOT NULL;

CREATE INDEX idx_list_steward ON list USING btree (steward_id);

-- Why a record was chosen for a Shortlist, as its chooser wrote it.
ALTER TABLE list_member ADD COLUMN note text;

-- One row per membership change of a Shortlist: who added or removed which
-- record, when, why, and the note they left. A removal the record's own
-- archive caused is written in the archive's transaction.
CREATE TABLE list_member_event (
    id uuid DEFAULT uuidv7() NOT NULL,
    list_id uuid NOT NULL,
    entity_type text NOT NULL,
    entity_id uuid NOT NULL,
    action text NOT NULL,
    -- What caused the change: a person's choice, a bulk change, or the
    -- record's own archive or restore.
    reason text NOT NULL,
    -- The typed principal id, spelled as list_member.added_by is.
    actor text NOT NULL,
    note text,
    occurred_at timestamptz DEFAULT now() NOT NULL,

    CONSTRAINT list_member_event_pkey PRIMARY KEY (id),
    CONSTRAINT list_member_event_list_id_fkey FOREIGN KEY (list_id) REFERENCES list(id) ON DELETE CASCADE,
    CONSTRAINT list_member_event_entity_type_check CHECK (entity_type IN ('contact', 'company', 'deal', 'lead', 'project')),
    CONSTRAINT list_member_event_action_check CHECK (action IN ('added', 'removed')),
    CONSTRAINT list_member_event_reason_check CHECK (reason IN ('chosen', 'bulk', 'record_archived', 'record_restored'))
);

CREATE INDEX idx_list_member_event_list ON list_member_event USING btree (list_id, occurred_at DESC, id DESC);
CREATE INDEX idx_list_member_event_entity ON list_member_event USING btree (entity_type, entity_id);

-- One row per version of a list's definition: the name, purpose, filter and
-- sharing it had from that version on, and who set them.
CREATE TABLE list_revision (
    id uuid DEFAULT uuidv7() NOT NULL,
    list_id uuid NOT NULL,
    version bigint NOT NULL,
    name text NOT NULL,
    purpose text,
    definition jsonb,
    sharing text NOT NULL,
    team_id uuid,
    steward_id uuid,
    changed_by text NOT NULL,
    changed_at timestamptz DEFAULT now() NOT NULL,

    CONSTRAINT list_revision_pkey PRIMARY KEY (id),
    CONSTRAINT list_revision_list_id_fkey FOREIGN KEY (list_id) REFERENCES list(id) ON DELETE CASCADE,
    CONSTRAINT list_revision_version_key UNIQUE (list_id, version)
);
