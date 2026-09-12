-- A closed deal can carry a review of how it went.
--
-- The review hangs off the CLOSING, not off the deal. A deal can be closed,
-- reopened and closed again, and each of those closings is a different thing to
-- review: a loss in March and a win in June are two outcomes, and a review
-- written about the first must not silently reattach itself to the second.
--
-- deal_stage_history already records every move, so the row that put the deal
-- into a terminal stage IS the closing occurrence. Nothing new is invented to
-- identify it. What is added is the unique key that lets another table name one
-- of those rows AND the deal it belongs to in a single foreign key, so a review
-- cannot point at a closing that happened on somebody else's deal.

SET LOCAL lock_timeout = '3s';

ALTER TABLE deal_stage_history ADD CONSTRAINT deal_stage_history_deal_id_id_key
    UNIQUE (deal_id, id);

-- What the move MEANT, frozen on the row that made it.
--
-- A stage's semantic is configuration and an administrator may change it: a
-- stage that meant "won" in March can be edited to mean "lost" in June, and the
-- deals already closed through it keep their own status. Deriving the outcome
-- by joining the live stage would then read those closings as something they
-- were not — or lose them entirely, when the deal's status and the stage's
-- current semantic stop agreeing.
--
-- Nullable, because every row written before this column existed has no answer
-- and inventing one would be the same mistake in the other direction. A closing
-- with no frozen semantic is one nothing can review, which is honest: the
-- record cannot say what that move meant.
ALTER TABLE deal_stage_history ADD COLUMN semantic_at_change text;

ALTER TABLE deal_stage_history ADD CONSTRAINT deal_stage_history_semantic_at_change_check
    CHECK (semantic_at_change IS NULL OR semantic_at_change IN ('open', 'won', 'lost'));

COMMENT ON COLUMN deal_stage_history.semantic_at_change IS
    'What the target stage meant WHEN the deal moved there. Null on rows written before this column existed. A stage semantic is configuration and can be edited afterwards; what the deal did cannot.';

-- The questions a review asks, versioned.
--
-- A template is EDITABLE, and that is the whole reason responses freeze their
-- questions rather than pointing at these rows. Changing the wording of a
-- question here must not change what a review written last quarter appears to
-- have asked, and retiring a template must not make old reviews unreadable.
CREATE TABLE activity_review_template (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    key text NOT NULL,
    label text NOT NULL,
    -- Which outcome this template is for. A win review and a loss review ask
    -- different questions, and offering both at a close is asking the human to
    -- do the filing the record can do itself.
    outcome text NOT NULL,
    -- The questions, as an ordered array of objects. Bounded by shape rather
    -- than normalized into their own table: nothing queries across questions,
    -- and two tables that must be read together to mean anything are one table
    -- wearing a join.
    questions jsonb NOT NULL,
    version integer NOT NULL DEFAULT 1,
    active boolean NOT NULL DEFAULT true,
    system boolean NOT NULL DEFAULT false,
    source text NOT NULL,
    captured_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    archived_at timestamptz,

    CONSTRAINT activity_review_template_key_shape
        CHECK (key ~ '^[a-z][a-z0-9_]{0,62}$'),
    CONSTRAINT activity_review_template_label_shape
        CHECK (btrim(label) <> '' AND char_length(label) <= 200),
    CONSTRAINT activity_review_template_outcome_check
        CHECK (outcome IN ('won', 'lost')),
    -- An array, and a non-empty one: a template asking nothing is a form with
    -- no fields, which reads as a bug to whoever opens it.
    CONSTRAINT activity_review_template_questions_shape
        CHECK (jsonb_typeof(questions) = 'array' AND jsonb_array_length(questions) > 0)
);

CREATE UNIQUE INDEX activity_review_template_live_key
    ON activity_review_template (key) WHERE archived_at IS NULL;

CREATE TRIGGER trg_activity_review_template_updated
    BEFORE UPDATE ON activity_review_template
    FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();

-- One filled-in review.
--
-- It is attached to an ACTIVITY, because a review is something a person wrote
-- and the activity is where the product already keeps those: it carries the
-- author, the time, the audience rules and the erasure treatment, and a second
-- store of human prose beside it would need all of that again.
--
-- The frozen columns are the point of the table. template_key, template_version
-- and questions record what was ASKED, copied at submission, so a later edit to
-- the template cannot change the meaning of an answer already given.
CREATE TABLE activity_review_response (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    -- One review per activity: the note IS the review, not a note that happens
    -- to have one attached.
    activity_id uuid NOT NULL UNIQUE REFERENCES activity(id) ON DELETE CASCADE,
    deal_id uuid NOT NULL REFERENCES deal(id) ON DELETE CASCADE,
    closing_occurrence_id uuid NOT NULL,
    -- The outcome as the CLOSING recorded it, not as the deal's stage label
    -- reads today. A stage can be renamed or reconfigured; what the deal did
    -- cannot.
    outcome text NOT NULL,
    template_key text NOT NULL,
    template_version integer NOT NULL,
    questions jsonb NOT NULL,
    answers jsonb NOT NULL,
    revision integer NOT NULL DEFAULT 1,
    -- The client's own id for this submission, unique per author. A retry of
    -- the same submission finds the existing response instead of writing a
    -- second review of one closing.
    submission_id uuid NOT NULL,
    source text NOT NULL,
    captured_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    -- The composite FK: the occurrence and the deal are checked together, so a
    -- review cannot name a closing that belongs to a different deal.
    CONSTRAINT activity_review_response_occurrence_fkey
        FOREIGN KEY (deal_id, closing_occurrence_id)
        REFERENCES deal_stage_history (deal_id, id) ON DELETE CASCADE,
    CONSTRAINT activity_review_response_outcome_check
        CHECK (outcome IN ('won', 'lost')),
    CONSTRAINT activity_review_response_questions_shape
        CHECK (jsonb_typeof(questions) = 'array' AND jsonb_array_length(questions) > 0),
    CONSTRAINT activity_review_response_answers_shape
        CHECK (jsonb_typeof(answers) = 'object'),
    CONSTRAINT activity_review_response_revision_check CHECK (revision >= 1)
);

-- A submission id is unique to its author, so two people can retry
-- independently and neither can overwrite the other's review.
CREATE UNIQUE INDEX activity_review_response_submission
    ON activity_review_response (captured_by, submission_id);

CREATE INDEX activity_review_response_by_occurrence
    ON activity_review_response (deal_id, closing_occurrence_id);

CREATE TRIGGER trg_activity_review_response_updated
    BEFORE UPDATE ON activity_review_response
    FOR EACH ROW EXECUTE FUNCTION set_updated_at_bump_version();

-- The two seeded templates. system = true, so the catalogue admin can retire
-- them but the product always ships with something to ask.
INSERT INTO activity_review_template (key, label, outcome, questions, system, source, captured_by)
VALUES
    ('win_review', 'Win review', 'won',
     '[{"key":"why_we_won","label":"Why did we win?","type":"text","required":true},
       {"key":"competitors","label":"Who else were they considering?","type":"text","required":false},
       {"key":"decisive_factor","label":"What decided it?","type":"text","required":false}]'::jsonb,
     true, 'system', 'system'),
    ('loss_review', 'Loss review', 'lost',
     '[{"key":"why_we_lost","label":"Why did we lose?","type":"text","required":true},
       {"key":"lost_to","label":"Who did they choose instead?","type":"text","required":false},
       {"key":"what_would_change_it","label":"What would have changed the outcome?","type":"text","required":false}]'::jsonb,
     true, 'system', 'system');
