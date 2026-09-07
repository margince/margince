-- A stage's exit criteria: what must be true of a deal before it leaves.
--
-- Configuration, not observation. A criterion says "the buyer confirmed the
-- problem" is a thing this stage requires; whether any particular deal has
-- met it is evidence, which lives elsewhere and cites its source. Keeping the
-- two apart is what lets a criterion be edited without rewriting history.
--
-- A TERMINAL stage carries none. Won and lost are where a deal stops, so
-- "what it takes to leave" has no referent there, and a criterion on one
-- would describe an exit that never happens. The CHECK cannot see the stage
-- row, so the store enforces it and TestATerminalStageCarriesNoExitCriteria
-- holds it.
--
-- ARCHIVED, never deleted. Evidence rows cite a criterion, and a reader
-- opening last quarter's deal must still see what the stage asked for at the
-- time. The unique key is therefore partial: a key freed by archiving can be
-- taken again, and both rows survive.
--
-- No workspace column and no policy: one installation holds one organization.
SET LOCAL lock_timeout = '5s';

CREATE TABLE stage_exit_criterion (
    id uuid DEFAULT uuidv7() NOT NULL,
    stage_id uuid NOT NULL,
    key text NOT NULL,
    label text NOT NULL,
    kind text NOT NULL,
    required boolean DEFAULT true NOT NULL,
    hint text,
    "position" integer NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    archived_at timestamptz,
    CONSTRAINT stage_exit_criterion_pkey PRIMARY KEY (id),
    CONSTRAINT stage_exit_criterion_kind_check
      CHECK (kind IN ('buyer_confirmed', 'event_held', 'document_signed',
                      'role_identified', 'terms_accepted', 'custom')),
    -- The key is what an extractor cites and what a criterion is matched by
    -- across an edit, so it is machine-shaped and bounded. The label is what
    -- a human reads and may say anything.
    CONSTRAINT stage_exit_criterion_key_shape CHECK (key ~ '^[a-z][a-z0-9_]{0,63}$'),
    CONSTRAINT stage_exit_criterion_label_len CHECK (length(label) BETWEEN 1 AND 120),
    CONSTRAINT stage_exit_criterion_hint_len CHECK (hint IS NULL OR length(hint) <= 500),
    CONSTRAINT stage_exit_criterion_position_check CHECK ("position" >= 0),
    CONSTRAINT stage_exit_criterion_stage_fkey
      FOREIGN KEY (stage_id) REFERENCES stage(id) ON DELETE CASCADE
);

-- Partial, so archiving frees the key for reuse while the old row stays
-- readable beside the evidence that cites it.
CREATE UNIQUE INDEX stage_exit_criterion_stage_key_live_ux
  ON stage_exit_criterion USING btree (stage_id, key)
  WHERE archived_at IS NULL;

CREATE INDEX stage_exit_criterion_stage_ix
  ON stage_exit_criterion USING btree (stage_id);

-- The version an If-Match compares against. Without this trigger the column
-- holds its default forever, two editors both send If-Match: 1, and the second
-- silently overwrites the first — the write the version check exists to refuse.
-- stage and pipeline carry the same trigger; a versioned table without one is
-- a precondition that cannot fail.
CREATE TRIGGER trg_stage_exit_criterion_updated
  BEFORE UPDATE ON stage_exit_criterion
  FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();

COMMENT ON COLUMN stage_exit_criterion.key IS
  'The stable machine name an evidence extractor cites. Unique among a stage''s live criteria; freed for reuse by archiving.';
COMMENT ON COLUMN stage_exit_criterion.kind IS
  'What KIND of fact settles this criterion. It decides which evidence sources may satisfy it — a buyer milestone is never settled by the seller''s own mail.';
COMMENT ON COLUMN stage_exit_criterion.required IS
  'Whether a deal must meet this criterion to leave the stage. An optional criterion is shown and gathered but never blocks.';

GRANT SELECT,INSERT,DELETE,UPDATE ON TABLE stage_exit_criterion TO margince_app;
