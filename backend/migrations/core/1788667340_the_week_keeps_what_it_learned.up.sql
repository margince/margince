-- What a week taught, kept beside the week it came from.
--
-- The narrative says what the week WAS. These say what to do differently, and
-- that is a different kind of claim: a description a reader can check against
-- the counts in front of them, versus advice they cannot. So every learning
-- carries CITATIONS into the week's own rows, and one that cites nothing is
-- refused rather than shown unsourced.
--
-- WRITE-ONCE. A learning rewritten next Tuesday is not the same claim the rep
-- read on Monday, and a retrospective whose lessons move is not a record. The
-- state column is what makes a second pass a no-op instead of a rewrite.
--
-- THREE STATES, and the middle one is why the column exists at all:
--   not_run              — no pass has looked. The lane may be unbound, the
--                          budget exhausted, the provider down.
--   insufficient_evidence — a pass ran, read the week, and had too little to
--                          say. This is an ANSWER, not a failure.
--   synthesized          — a pass ran and wrote what is in the rows below.
-- A reader that cannot tell the first two apart tells a rep "nothing to learn"
-- about a week nobody looked at.
SET LOCAL lock_timeout = '3s';

ALTER TABLE weekly_review
    ADD COLUMN learnings_state text NOT NULL DEFAULT 'not_run',
    ADD COLUMN learned_at timestamptz;

ALTER TABLE weekly_review
    ADD CONSTRAINT weekly_review_learnings_state_check
    CHECK (learnings_state IN ('not_run', 'insufficient_evidence', 'synthesized'));

-- A pass that ran left a stamp; one that did not, did not. Spelled with
-- IS NOT DISTINCT FROM because a CHECK evaluating to NULL PASSES in Postgres,
-- so `= 'not_run'` would admit exactly the rows this refuses.
ALTER TABLE weekly_review
    ADD CONSTRAINT weekly_review_learned_at_matches_state
    CHECK ((learnings_state IS NOT DISTINCT FROM 'not_run') = (learned_at IS NULL));

-- One thing the week taught.
CREATE TABLE weekly_review_learning (
    id uuid DEFAULT uuidv7() NOT NULL,
    weekly_review_id uuid NOT NULL,
    -- What sort of claim this is. A closed vocabulary because the surface draws
    -- each differently and a reader learns the four shapes.
    kind text NOT NULL,
    -- The claim itself, in the rep's own reading language.
    text text NOT NULL,
    -- Where it sits in the list the pass produced. Order is the pass's, and it
    -- is kept because the first learning is the one a rep reads.
    position integer NOT NULL,
    created_at timestamptz DEFAULT now() NOT NULL,

    CONSTRAINT weekly_review_learning_pkey PRIMARY KEY (id),
    CONSTRAINT weekly_review_learning_review_fkey
        FOREIGN KEY (weekly_review_id) REFERENCES weekly_review(id) ON DELETE CASCADE,
    CONSTRAINT weekly_review_learning_kind_check
        CHECK (kind IN ('worked', 'did_not_work', 'pattern', 'experiment')),
    CONSTRAINT weekly_review_learning_text_present CHECK (btrim(text) <> ''),
    -- 400 characters. A learning is one claim a rep reads in a glance; past
    -- this it is an essay, and the Go seam bounds it to the same number.
    CONSTRAINT weekly_review_learning_text_bound CHECK (length(text) <= 400),
    CONSTRAINT weekly_review_learning_position_sane CHECK (position >= 0),
    -- One learning per slot per review, so a replayed write cannot double the
    -- list.
    CONSTRAINT uq_weekly_review_learning_slot UNIQUE (weekly_review_id, position)
);

-- What one learning was drawn FROM.
--
-- A separate table rather than a jsonb column because the citations are
-- CHECKED: every one must name a row the pass was actually shown, and a reply
-- citing anything else is refused whole. Rows are what a foreign key and a
-- count can speak about; a blob is not.
CREATE TABLE weekly_review_learning_citation (
    id uuid DEFAULT uuidv7() NOT NULL,
    learning_id uuid NOT NULL,
    -- What kind of row this points at, and which one. NO foreign key to the
    -- subject: a citation must survive the deal it names being deleted, exactly
    -- as the review's own frozen deal lines do. A dangling id is honest about
    -- where the claim came from.
    subject_type text NOT NULL,
    subject_id uuid NOT NULL,
    -- What the subject was CALLED when the learning was written, so a citation
    -- still reads after a rename.
    label text NOT NULL,

    CONSTRAINT weekly_review_learning_citation_pkey PRIMARY KEY (id),
    CONSTRAINT weekly_review_learning_citation_learning_fkey
        FOREIGN KEY (learning_id) REFERENCES weekly_review_learning(id) ON DELETE CASCADE,
    CONSTRAINT weekly_review_learning_citation_subject_check
        CHECK (subject_type IN ('deal', 'commitment')),
    CONSTRAINT weekly_review_learning_citation_label_present CHECK (btrim(label) <> ''),
    -- The same subject cited twice by one learning says nothing twice.
    CONSTRAINT uq_weekly_review_learning_citation
        UNIQUE (learning_id, subject_type, subject_id)
);

CREATE INDEX idx_weekly_review_learning_review
    ON weekly_review_learning (weekly_review_id, position);
