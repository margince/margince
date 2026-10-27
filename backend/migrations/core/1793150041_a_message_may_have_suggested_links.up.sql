-- A suggestion that a message belongs to a record, supplied by an extension and
-- waiting for a person. Nothing here attaches the message: only accepting a
-- suggestion writes the activity_link, through the same relink every other
-- filing uses.
--
-- The row holds no subject data and no figure about a person. It names the
-- message, the target, the unit that proposed it and one short reason. A
-- suggestion the author of the message would be the target of is refused before
-- it is written, so the table never carries one.
--
-- entity_id has no foreign key because the target is one of four tables. A
-- target that is gone makes the accept fail as not found; the row stays pending
-- until someone rejects it.
SET LOCAL lock_timeout = '5s';

CREATE TABLE activity_link_suggestion (
    id uuid DEFAULT uuidv7() NOT NULL,
    activity_id uuid NOT NULL,
    entity_type text NOT NULL,
    entity_id uuid NOT NULL,
    unit text NOT NULL,
    reason text NOT NULL,
    score numeric(5,4) NOT NULL,
    status text DEFAULT 'pending'::text NOT NULL,
    decided_by text,
    decided_at timestamptz,
    captured_by text NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT activity_link_suggestion_pkey PRIMARY KEY (id),
    CONSTRAINT activity_link_suggestion_activity_id_fkey FOREIGN KEY (activity_id)
        REFERENCES activity(id) ON DELETE CASCADE,
    CONSTRAINT activity_link_suggestion_entity_type_check
        CHECK (entity_type IN ('contact', 'company', 'deal', 'project')),
    CONSTRAINT activity_link_suggestion_status_check
        CHECK (status IN ('pending', 'accepted', 'rejected')),
    CONSTRAINT activity_link_suggestion_score_check CHECK (score >= 0 AND score <= 1),
    CONSTRAINT activity_link_suggestion_reason_check
        CHECK (char_length(reason) BETWEEN 1 AND 200),
    CONSTRAINT activity_link_suggestion_version_check
        CHECK (version BETWEEN -9007199254740991 AND 9007199254740991),
    CONSTRAINT activity_link_suggestion_decided CHECK (
        (status = 'pending' AND decided_at IS NULL AND decided_by IS NULL)
        OR (status <> 'pending' AND decided_at IS NOT NULL AND decided_by IS NOT NULL))
);

-- A repeat proposal of the same target answers the row already there, whatever
-- its status, so a rejected suggestion does not come back.
CREATE UNIQUE INDEX uq_activity_link_suggestion_target
    ON activity_link_suggestion (activity_id, entity_type, entity_id);
CREATE INDEX idx_activity_link_suggestion_pending
    ON activity_link_suggestion (created_at, id) WHERE status = 'pending';

CREATE TRIGGER activity_link_suggestion_touch BEFORE UPDATE ON activity_link_suggestion
    FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE activity_link_suggestion TO margince_app;
