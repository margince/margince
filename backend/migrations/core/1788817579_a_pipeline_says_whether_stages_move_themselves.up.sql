-- A pipeline's answer, per transition, to whether stages move themselves.
--
-- One row per (pipeline, from, to). NOT per rep: whether a transition is
-- trustworthy is a fact about the transition and the evidence behind it, not
-- about who owns the deal — two reps working the same stage would otherwise
-- get different answers from the same measured record, and the rates that
-- decide it are counted per transition (stage_progression_outcome).
--
-- The thresholds live on the ROW rather than in code because they are what an
-- installation argues about. A conservative customer raises min_reviewed; a
-- fast-moving one shortens the window. The defaults here are the product's
-- opinion, not a limit.

SET LOCAL lock_timeout = '3s';

CREATE TABLE stage_progression_policy (
    id uuid DEFAULT uuidv7() NOT NULL,
    -- NO workspace_id, and that is the convention rather than an omission: the
    -- tenant boundary reaches this table through pipeline_id, and every read
    -- goes through database.WithWorkspaceTx. A column here would be a second
    -- answer to which tenant owns the row, and the two would drift.
    pipeline_id uuid NOT NULL,
    from_stage_id uuid NOT NULL,
    to_stage_id uuid NOT NULL,
    -- What an admin has ASKED for. Asking is not the same as being allowed:
    -- 'auto' is a request that the thresholds below still have to satisfy, and
    -- the apply path re-checks them every time rather than trusting this word.
    mode text DEFAULT 'propose' NOT NULL,
    -- The bar, as this installation sets it.
    clean_acceptance_threshold numeric(4,3) DEFAULT 0.950 NOT NULL,
    correction_reversal_threshold numeric(4,3) DEFAULT 0.010 NOT NULL,
    min_reviewed integer DEFAULT 200 NOT NULL,
    min_observation_days integer DEFAULT 28 NOT NULL,
    window_days integer DEFAULT 30 NOT NULL,
    -- How long a person has to take an automatic move back. After it, the
    -- move is ordinary history and the undo route refuses.
    undo_window_hours integer DEFAULT 72 NOT NULL,
    enabled_by uuid,
    enabled_at timestamptz,
    -- A rule the product turned off ITSELF, because the record went bad. Kept
    -- apart from mode: an admin who set 'auto' should find their setting still
    -- there when the suspension lifts, rather than discover the product
    -- silently rewrote what they asked for.
    suspended_at timestamptz,
    suspended_reason text,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    CONSTRAINT stage_progression_policy_pkey PRIMARY KEY (id),
    CONSTRAINT stage_progression_policy_mode CHECK (mode IN ('propose', 'auto')),
    -- A suspension is a whole fact: a reason with no instant cannot be placed
    -- on a timeline, and an instant with no reason tells an admin their
    -- automation stopped and nothing about why.
    CONSTRAINT stage_progression_policy_suspension_whole
      CHECK ((suspended_at IS NULL) = (suspended_reason IS NULL)),
    -- Enabling names who did it. enabled_by is ON DELETE SET NULL like every
    -- other app_user reference here, so the pair is not required in both
    -- directions: an instant whose person has since been erased is what an
    -- anonymized workspace looks like, and the audit trail still holds who.
    CONSTRAINT stage_progression_policy_enabling_is_timed
      CHECK (enabled_by IS NULL OR enabled_at IS NOT NULL),
    -- Rates are shares. A threshold outside 0..1 is a typo that would either
    -- enable everything or nothing, and both fail quietly.
    CONSTRAINT stage_progression_policy_thresholds_are_shares
      CHECK (clean_acceptance_threshold >= 0 AND clean_acceptance_threshold <= 1
             AND correction_reversal_threshold >= 0
             AND correction_reversal_threshold <= 1),
    -- A bar of zero reviewed proposals is no bar: it would enable a transition
    -- the moment it was configured, on no evidence at all.
    CONSTRAINT stage_progression_policy_volume_is_a_bar
      CHECK (min_reviewed > 0 AND min_observation_days > 0
             AND window_days > 0 AND undo_window_hours > 0),
    CONSTRAINT stage_progression_policy_pipeline_fkey
      FOREIGN KEY (pipeline_id) REFERENCES pipeline(id) ON DELETE CASCADE,
    CONSTRAINT stage_progression_policy_from_stage_fkey
      FOREIGN KEY (from_stage_id) REFERENCES stage(id) ON DELETE CASCADE,
    CONSTRAINT stage_progression_policy_to_stage_fkey
      FOREIGN KEY (to_stage_id) REFERENCES stage(id) ON DELETE CASCADE,
    CONSTRAINT stage_progression_policy_enabled_by_fkey
      FOREIGN KEY (enabled_by) REFERENCES app_user(id) ON DELETE SET NULL
);

-- One rule per transition. Two rows for one transition would be two answers to
-- "may this move itself", and the apply path would take whichever the planner
-- happened to return first.
CREATE UNIQUE INDEX stage_progression_policy_transition_key
    ON stage_progression_policy USING btree (pipeline_id, from_stage_id, to_stage_id);

COMMENT ON TABLE stage_progression_policy IS
    'Per-transition governance for automatic stage moves. mode is what an admin ASKED for; the thresholds are what the apply path re-checks every time.';
COMMENT ON COLUMN stage_progression_policy.suspended_at IS
    'Set by the product when the measured record goes bad, never by an admin. Kept apart from mode so an admin''s setting survives a suspension.';
