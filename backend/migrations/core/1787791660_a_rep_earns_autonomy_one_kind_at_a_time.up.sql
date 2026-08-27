-- What each rep has decided about letting a KIND of proposal apply itself, and
-- the track record that decision was earned on.
--
-- WHY PER USER AND PER KIND.
--
-- Trust is not a property of the software; it is a property of one person's
-- experience of one kind of proposal. A rep who has approved fourteen
-- close-date confirmations unchanged has evidence about close dates and none
-- about outbound mail, and a policy keyed to anything coarser would carry the
-- first rep's confidence into the second rep's inbox. So the grain is the pair,
-- and there is exactly one row per pair.
--
-- WHY THE COUNTERS LIVE HERE AND NOT IN A QUERY OVER approval.
--
-- They could be counted from the approval table, and that was the first design.
-- It is wrong for one reason that only shows up later: approvals expire and are
-- swept, and a retention policy will eventually delete decided rows. A track
-- record derived from a table that forgets would silently reset a rep's earned
-- standing when housekeeping ran. The counters are therefore a durable fact of
-- their own, incremented as each decision is made.
--
-- WHAT THIS TABLE DOES NOT DO.
--
-- It does not execute anything. `mode` records what a rep has chosen and the
-- counters record what they have done; no writer in this migration's product
-- reads `mode` to skip asking. Auto-apply needs an execution route that does
-- not exist yet — approvals are decided by people (decidingactor.go refuses a
-- system principal outright), and until that is answered a stored 'auto' would
-- be a claim the product cannot honour. The value is in the CHECK because the
-- ladder's rungs are one vocabulary and splitting them across two migrations
-- would let the second disagree with the first.
CREATE TABLE approval_autonomy_policy (
    id uuid DEFAULT uuidv7() NOT NULL,
    user_id uuid NOT NULL,
    -- The approval kind, as the server spells it: 'close_date_correction',
    -- 'held_draft'. Not a foreign key, because the kinds are a Go vocabulary
    -- (approvals.StageableKinds) rather than a table — the same reason
    -- approval.kind is text. A row naming a kind the server has retired is
    -- inert rather than wrong: nothing stages it, so nothing reads the policy.
    kind text NOT NULL,
    -- manual: ask every time. The default, and what every rep has until they
    --         choose otherwise.
    -- veto:   apply after a stated delay unless the rep stops it first.
    -- auto:   apply on sight, undoably.
    --
    -- Stored, never yet acted on. See the note above.
    mode text DEFAULT 'manual' NOT NULL,
    -- How long a veto row waits before it would apply. NULL for the other two
    -- modes, which have no window: 'manual' never applies and 'auto' does not
    -- wait. A window on a mode that cannot use one is a number that looks like
    -- a promise, so the CHECK below refuses it.
    veto_window interval,
    -- The track record, each counting a decision this rep made about this kind.
    --
    -- 'clean' is the one that earns promotion: approved with the payload the
    -- server proposed, untouched. An edit is a correction — the rep agreed with
    -- the intent and disagreed with the detail — so it is counted apart rather
    -- than folded into either approval or rejection. Reading it as an approval
    -- would offer autonomy to a kind the rep rewrites every time, which is the
    -- exact case where the software should keep asking.
    approved_clean integer DEFAULT 0 NOT NULL,
    approved_edited integer DEFAULT 0 NOT NULL,
    rejected integer DEFAULT 0 NOT NULL,
    -- When the rep last moved up or down the ladder. Both nullable because a
    -- row that has only ever counted decisions has done neither, and 'never
    -- promoted' is a different fact from 'promoted at the epoch'.
    promoted_at timestamptz,
    demoted_at timestamptz,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT approval_autonomy_policy_mode_check
        CHECK (mode IN ('manual', 'veto', 'auto')),
    -- A window belongs to the mode that waits, and to no other.
    CONSTRAINT approval_autonomy_policy_window_shape
        CHECK ((mode = 'veto' AND veto_window IS NOT NULL)
            OR (mode <> 'veto' AND veto_window IS NULL)),
    -- A window of zero or less is 'auto' spelled as a wait, and a negative one
    -- is a deadline in the past. Both would apply immediately while reading as
    -- a chance to intervene.
    CONSTRAINT approval_autonomy_policy_window_positive
        CHECK (veto_window IS NULL OR veto_window > interval '0'),
    -- A count of decisions cannot run backwards.
    CONSTRAINT approval_autonomy_policy_counts_nonnegative
        CHECK (approved_clean >= 0 AND approved_edited >= 0 AND rejected >= 0),
    -- The kind is server vocabulary, and this is the grammar it is written in.
    -- Without it a caller could store ' close_date_correction' or an empty
    -- string, and the row would then match no kind the server ever stages while
    -- looking exactly like a policy somebody set.
    CONSTRAINT approval_autonomy_policy_kind_grammar
        CHECK (kind ~ '^[a-z][a-z0-9_]*$' AND char_length(kind) <= 64)
);

ALTER TABLE approval_autonomy_policy
    ADD CONSTRAINT approval_autonomy_policy_pkey PRIMARY KEY (id);

-- One policy per rep per kind. Two rows would let a reader take whichever it
-- found first, which for a setting about automatic action is the difference
-- between asking and not asking.
ALTER TABLE approval_autonomy_policy
    ADD CONSTRAINT uq_approval_autonomy_policy_user_kind UNIQUE (user_id, kind);

-- CASCADE on the rep, matching agent_standing_grant and brief_run: a departed
-- colleague's autonomy setting is not a record anybody needs, and a policy row
-- pointing at a user who cannot answer for it is an orphan that still reads as
-- somebody's standing choice.
ALTER TABLE approval_autonomy_policy
    ADD CONSTRAINT approval_autonomy_policy_user_fkey
    FOREIGN KEY (user_id) REFERENCES app_user(id) ON DELETE CASCADE;

-- The stamp is kept by the trigger rather than by each writer. A counter is
-- bumped from more than one place, and a column every writer must remember is
-- one a writer eventually forgets — silently, since a stale updated_at looks
-- exactly like a row nobody touched.
CREATE TRIGGER trg_approval_autonomy_policy_updated
    BEFORE UPDATE ON approval_autonomy_policy
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();
