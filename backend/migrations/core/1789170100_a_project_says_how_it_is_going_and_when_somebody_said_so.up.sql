SET LOCAL lock_timeout = '3s';

-- How a project is going, as a person judged it on a day.
--
-- Append-only, and that is the whole design. A `health` column on project would
-- answer "how is it now" and destroy the answer to "how was it in March", which
-- is the question a delivery review actually asks. Every assessment stays; the
-- current one is derived.
--
-- A mistake is corrected by SUPERSEDING the row, never by editing it: the
-- successor carries the target's effective time, so a correction fixes what was
-- said without moving when it was said. Editing in place would let a project's
-- recorded history change under a reader who had already acted on it.
CREATE TABLE project_health_assessment (
    id          uuid PRIMARY KEY DEFAULT uuidv7(),
    project_id  uuid NOT NULL REFERENCES project(id) ON DELETE CASCADE,

    state       text NOT NULL,
    note        text,

    -- WHEN the judgement applies, which is not when the row was written: a lead
    -- catching up on Monday records Friday's reading, and the timeline has to
    -- show it on Friday.
    assessed_at timestamptz NOT NULL,

    source      text NOT NULL,
    -- Who is on record as having judged it, when that is not the person typing:
    -- a lead entering what a delivery manager said in a meeting names them here
    -- while captured_by stays the authenticated session.
    source_author text,
    -- text, not a key into app_user: captured_by holds a PREFIXED principal
    -- ("human:<id>" / "agent:<id>"), and an agent acting under a passport is
    -- not a row in app_user at all.
    captured_by text NOT NULL,

    created_at  timestamptz NOT NULL DEFAULT now(),

    supersedes_assessment_id uuid,

    CONSTRAINT project_health_state_known
        CHECK (state IN ('on_track', 'at_risk', 'off_track')),
    -- A note is required for anything but on_track. "At risk" with no reason is
    -- an alarm nobody can act on, and the person who could say why is the one
    -- filing it.
    CONSTRAINT project_health_trouble_is_explained
        CHECK (state = 'on_track' OR (note IS NOT NULL AND length(btrim(note)) > 0)),
    CONSTRAINT project_health_note_bounded
        CHECK (note IS NULL OR length(note) <= 4000),
    -- A row cannot supersede itself, which would make it its own predecessor
    -- and hide it from every current-state read at once.
    CONSTRAINT project_health_not_its_own_successor
        CHECK (supersedes_assessment_id IS DISTINCT FROM id)
);

-- The composite key the correction FK points at, so a correction cannot name a
-- row belonging to a DIFFERENT project. A plain FK to (id) would admit it, and
-- the result reads as one project's history quietly rewriting another's.
ALTER TABLE project_health_assessment
    ADD CONSTRAINT project_health_assessment_project_id_key UNIQUE (project_id, id);

ALTER TABLE project_health_assessment
    ADD CONSTRAINT project_health_supersedes_same_project
    FOREIGN KEY (project_id, supersedes_assessment_id)
    REFERENCES project_health_assessment (project_id, id)
    ON DELETE RESTRICT;

-- One successor per row. Without this two corrections of one assessment both
-- stand, and "which reading is current" has two answers with nothing to choose
-- between them.
CREATE UNIQUE INDEX uq_project_health_one_successor
    ON project_health_assessment (supersedes_assessment_id)
    WHERE supersedes_assessment_id IS NOT NULL;

-- The order every read wants: newest judgement first, ties broken by when it
-- was written and then by id, so a page boundary lands in the same place twice.
CREATE INDEX idx_project_health_current
    ON project_health_assessment (project_id, assessed_at DESC, created_at DESC, id DESC);


-- Append-only in the DATABASE, not only in the Go that writes it.
--
-- Without this the runtime role can UPDATE an assessment in place, and every
-- constraint above still passes: the history a delivery review reads would
-- change under it with nothing to show that it had. Worse, updating a row to
-- supersede its own successor builds a cycle that satisfies the unique index
-- and the composite FK while leaving CurrentHealthTx with no current reading
-- at all — a project that silently reports as never judged.
--
-- A judgement is withdrawn by superseding it, which is an INSERT. Nothing in
-- this product edits one, so nothing legitimate is refused here.
CREATE FUNCTION project_health_assessment_is_append_only() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  -- The CASCADE from project is the one legitimate removal, and it is
  -- distinguishable: by the time this fires the parent is already gone, so a
  -- surviving project means somebody deleted the assessment directly.
  IF TG_OP = 'DELETE' THEN
    IF EXISTS (SELECT 1 FROM project p WHERE p.id = OLD.project_id) THEN
      RAISE EXCEPTION 'health assessment % is append-only: correct it by superseding it', OLD.id
        USING ERRCODE = 'check_violation',
              CONSTRAINT = 'project_health_assessment_append_only';
    END IF;
    RETURN OLD;
  END IF;
  RAISE EXCEPTION 'health assessment % is append-only: correct it by superseding it', OLD.id
    USING ERRCODE = 'check_violation',
          CONSTRAINT = 'project_health_assessment_append_only';
END;
$$;

CREATE TRIGGER project_health_assessment_append_only
    BEFORE DELETE OR UPDATE ON project_health_assessment
    FOR EACH ROW EXECUTE FUNCTION project_health_assessment_is_append_only();
