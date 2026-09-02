-- The week ahead, written down and answered.
--
-- weekly_review is the past: a frozen retrospective, written by a job, that
-- nobody edits. What it has no room for is the FUTURE — what a rep intends to
-- do about what it says, and where they are stuck.
--
-- So a plan is a different kind of record and gets its own module. It is
-- authored by a person, audited, evented and RBAC-gated, and it has a SECOND
-- writer: the rep's lead, answering a request for help. A retrospective has
-- none of those properties, which is why this does not become more columns on
-- the table beside it.
SET LOCAL lock_timeout = '5s';

CREATE TABLE weekly_plan (
    id uuid DEFAULT uuidv7() NOT NULL,
    owner_id uuid NOT NULL,
    -- The Monday of the week being planned, in the installation's reporting
    -- zone — the same shape and the same zone weekly_review.local_week_start
    -- uses. One spelling of "which week", so a plan and the review beside it
    -- line up.
    local_week_start date NOT NULL,
    -- open while the week runs; closed once the weekly job has frozen its
    -- outcomes into the review. A closed plan is history and stops accepting
    -- edits, which is what makes the review's counts stay true.
    status text DEFAULT 'open' NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    captured_by text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,

    -- What the week came to, STAMPED at close rather than recomputed.
    --
    -- The retrospective beside this freezes these two numbers, and the close
    -- runs more than once — the dispatcher ticks repeatedly inside a week so a
    -- worker that was down still backfills. Recounting live rows on the second
    -- pass would let a commitment that changed after the first close move the
    -- answer, and the review would stop matching the plan it describes.
    --
    -- NULL while the week is open: there is no outcome yet, and a zero would
    -- claim one.
    commitments_due int,
    commitments_kept int,

    CONSTRAINT weekly_plan_pkey PRIMARY KEY (id),
    CONSTRAINT weekly_plan_status_check CHECK (status IN ('open', 'closed')),
    -- A closed week has an outcome and an open one has none. The stamp and the
    -- status move together or the row says two different things about itself.
    CONSTRAINT weekly_plan_outcome_matches_status CHECK (
        (status = 'closed') = (commitments_due IS NOT NULL)
        AND (commitments_due IS NULL) = (commitments_kept IS NULL)),
    CONSTRAINT weekly_plan_outcome_is_a_tally CHECK (
        commitments_due IS NULL
        OR (commitments_due >= 0 AND commitments_kept >= 0
            AND commitments_kept <= commitments_due))
);

-- One plan per rep per week. The same arbiter weekly_review uses, and for the
-- same reason: two writers racing produce one plan and no error.
ALTER TABLE weekly_plan
    ADD CONSTRAINT uq_weekly_plan_owner_week UNIQUE (owner_id, local_week_start);

-- A departed rep's plans go with them, exactly as their reviews and briefs do.
ALTER TABLE weekly_plan
    ADD CONSTRAINT weekly_plan_owner_fkey FOREIGN KEY (owner_id)
    REFERENCES app_user(id) ON DELETE CASCADE;

CREATE INDEX idx_weekly_plan_owner ON weekly_plan (owner_id, local_week_start DESC);

-- One thing a rep said they would do this week.
CREATE TABLE weekly_plan_commitment (
    id uuid DEFAULT uuidv7() NOT NULL,
    plan_id uuid NOT NULL,
    label text NOT NULL,

    -- The record this commitment is about, when it names one.
    --
    -- NO FOREIGN KEY on linked_record_id, and that is deliberate — the same
    -- ruling weekly_review_deal makes. A commitment about a deal that is later
    -- deleted still says what the rep undertook; a foreign key would erase the
    -- promise along with its subject. The client resolves the label through the
    -- record's own endpoint, so a deleted record simply stops resolving.
    linked_record_type text,
    linked_record_id uuid,

    due_on date,
    state text DEFAULT 'open' NOT NULL,

    -- What the rep needs from their lead, and what the lead said.
    --
    -- Empty string rather than NULL for both: the question "did anyone ask for
    -- help" is answered by btrim(help_requested) <> '', one predicate, and a
    -- column that is sometimes NULL and sometimes '' makes that two.
    help_requested text DEFAULT '' NOT NULL,
    manager_response text DEFAULT '' NOT NULL,
    manager_user_id uuid,
    responded_at timestamptz,

    completed_at timestamptz,
    position int DEFAULT 0 NOT NULL,
    version bigint DEFAULT 1 NOT NULL,
    captured_by text NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,
    updated_at timestamptz DEFAULT now() NOT NULL,

    CONSTRAINT weekly_plan_commitment_pkey PRIMARY KEY (id),
    -- A commitment nobody can read is not a commitment.
    CONSTRAINT weekly_plan_commitment_label_present CHECK (btrim(label) <> ''),
    CONSTRAINT weekly_plan_commitment_label_bound CHECK (length(label) <= 500),
    CONSTRAINT weekly_plan_commitment_help_bound CHECK (length(help_requested) <= 2000),
    CONSTRAINT weekly_plan_commitment_response_bound CHECK (length(manager_response) <= 2000),
    CONSTRAINT weekly_plan_commitment_state_check CHECK (
        state IN ('open', 'done', 'missed', 'dropped')),
    CONSTRAINT weekly_plan_commitment_link_type_check CHECK (
        linked_record_type IS NULL OR
        linked_record_type IN ('deal', 'lead', 'person', 'organization', 'project')),
    -- Both or neither. A type with no id names nothing; an id with no type
    -- cannot be looked up in any table.
    CONSTRAINT weekly_plan_commitment_link_whole CHECK (
        (linked_record_type IS NULL) = (linked_record_id IS NULL)),
    -- A done commitment says when. The pair is what lets the weekly job count
    -- "kept this week" as a real window rather than an inference — the same
    -- rule activity_done_at holds for a task.
    CONSTRAINT weekly_plan_commitment_done_at CHECK (
        (state = 'done') = (completed_at IS NOT NULL)),
    -- A response is a person and a moment, or it is none of the three. A stored
    -- answer with nobody behind it cannot be shown to the rep who asked.
    CONSTRAINT weekly_plan_commitment_response_whole CHECK (
        (btrim(manager_response) = '') = (manager_user_id IS NULL)
        AND (manager_user_id IS NULL) = (responded_at IS NULL))
);

ALTER TABLE weekly_plan_commitment
    ADD CONSTRAINT weekly_plan_commitment_plan_fkey FOREIGN KEY (plan_id)
    REFERENCES weekly_plan(id) ON DELETE CASCADE;

-- SET NULL, not CASCADE: a lead who leaves must not delete the answer they
-- gave — the rep asked a question and got one, and that exchange is the record.
--
-- But response_whole ties the text, the name and the moment together, so a bare
-- SET NULL would leave a row the CHECK rejects. The trigger below clears all
-- three, which keeps the answer's SHAPE honest: what survives a departure is
-- that the commitment was answered, not a quotation attributed to nobody.
ALTER TABLE weekly_plan_commitment
    ADD CONSTRAINT weekly_plan_commitment_manager_fkey FOREIGN KEY (manager_user_id)
    REFERENCES app_user(id);

-- Clearing all three together is what keeps response_whole satisfiable. A
-- plain ON DELETE SET NULL nulls the id and leaves the text, which the CHECK
-- then refuses — so deleting a departed lead would fail on rows nobody is
-- looking at.
CREATE FUNCTION weekly_plan_commitment_forget_manager() RETURNS trigger
    LANGUAGE plpgsql AS $$
BEGIN
    UPDATE weekly_plan_commitment
       SET manager_response = '', manager_user_id = NULL, responded_at = NULL
     WHERE manager_user_id = OLD.id;
    RETURN OLD;
END;
$$;

CREATE TRIGGER app_user_forget_weekly_plan_responses
    BEFORE DELETE ON app_user
    FOR EACH ROW EXECUTE FUNCTION weekly_plan_commitment_forget_manager();

CREATE INDEX idx_weekly_plan_commitment_plan
    ON weekly_plan_commitment (plan_id, position, created_at);

-- The lead's read: which of my people asked for help this week. Partial, so it
-- indexes the few rows that asked rather than every commitment ever written.
CREATE INDEX idx_weekly_plan_commitment_help
    ON weekly_plan_commitment (plan_id)
    WHERE btrim(help_requested) <> '';

-- What the week's plan came to, frozen into the retrospective beside it.
ALTER TABLE weekly_review
    ADD COLUMN commitments_due int DEFAULT 0 NOT NULL,
    ADD COLUMN commitments_kept int DEFAULT 0 NOT NULL,
    ADD CONSTRAINT weekly_review_commitments_are_tallies CHECK (
        commitments_due >= 0 AND commitments_kept >= 0
        AND commitments_kept <= commitments_due);
