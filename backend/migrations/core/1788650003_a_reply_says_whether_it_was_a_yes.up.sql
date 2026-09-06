-- Whether an inbound reply was positive, negative or neither.
--
-- An SDR's headline number is positive-reply rate, and nothing in the product
-- recorded whether a reply was positive. The waiting queue knows somebody
-- answered; the attention label knows whether the message asks for something.
-- Neither says whether the answer was a yes, which is the whole of what the
-- number is about.
--
-- RIDES THE EXISTING CLASSIFY PASS. capture_classify already reads this exact
-- backlog and already spends one model call per ten messages; this verdict is
-- another field in that call's answer, not a second pass over the same mail. A
-- separate pass would double the per-message cost to re-read text the first one
-- had already been given.
--
-- WORKSPACE-GLOBAL, not per reader, for the reason owed_verdict states beside
-- it: whether a reply was positive is a property of the message, and a
-- per-reader verdict would be a second answer to the same question.
--
-- OUTBOUND MAIL IS NOT JUDGED. The classify backlog carries both directions
-- because a label routes attention on either. A reply verdict on a message the
-- workspace itself sent would be judging our own words as though they were a
-- customer's answer, so direction is part of the CHECK rather than left to the
-- writer to remember.
--
-- Existing rows keep NULL, which reads as UNJUDGED — and unjudged is a real
-- answer rather than a missing one. A rate computed over these rows counts an
-- unjudged reply in NEITHER its numerator nor its denominator, so a week of
-- mostly-unjudged mail reports a small denominator instead of a confident wrong
-- number. That is the decided behaviour, not an implementation convenience.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity
    ADD COLUMN IF NOT EXISTS reply_verdict text,
    ADD COLUMN IF NOT EXISTS reply_verdict_at timestamptz,
    -- The classifier version that judged it. A model change and the world
    -- changing look identical in a rate that moved; this column is what tells
    -- them apart afterwards. Free text rather than a foreign key: it records
    -- what answered at the time, and the answer outlives any registry row.
    ADD COLUMN IF NOT EXISTS reply_verdict_by text;

-- NOT VALID, then validated separately — the shape 1788489200 records for this
-- table and asks a later migration to copy.
--
-- A plain ADD CONSTRAINT ... CHECK scans every existing row while holding ACCESS
-- EXCLUSIVE, which on a table the size of activity locks out every reader and
-- writer for the length of the scan. lock_timeout does not help: it bounds how
-- long we WAIT for the lock, never how long we hold it.
ALTER TABLE activity
    ADD CONSTRAINT activity_reply_verdict_check
    CHECK (reply_verdict IS NULL OR reply_verdict IN ('positive', 'negative', 'neutral'))
    NOT VALID;

-- Three values, not four. `unknown` is the ABSENCE of a verdict and is spelled
-- NULL, exactly as owed_verdict spells its unjudged state: a fourth enum value
-- would let a row claim the classifier reached "unknown" as a conclusion when
-- what happened is that it never reached the floor. One state, one spelling.
ALTER TABLE activity
    ADD CONSTRAINT activity_reply_verdict_stamped
    CHECK ((reply_verdict IS NULL) = (reply_verdict_at IS NULL)
       AND (reply_verdict IS NULL) = (reply_verdict_by IS NULL))
    NOT VALID;

-- A verdict belongs to an inbound message. Held here rather than in the writer,
-- because a writer's memory is not an invariant.
ALTER TABLE activity
    ADD CONSTRAINT activity_reply_verdict_inbound
    CHECK (reply_verdict IS NULL OR direction = 'inbound')
    NOT VALID;

ALTER TABLE activity VALIDATE CONSTRAINT activity_reply_verdict_check;
ALTER TABLE activity VALIDATE CONSTRAINT activity_reply_verdict_stamped;
ALTER TABLE activity VALIDATE CONSTRAINT activity_reply_verdict_inbound;

COMMENT ON COLUMN activity.reply_verdict IS
    'Whether this inbound reply was positive, negative or neutral. NULL means unjudged, which enters neither half of a positive-reply rate.';
COMMENT ON COLUMN activity.reply_verdict_by IS
    'The classifier version that judged it, so a model change is distinguishable from the world changing.';

-- The correction history, append-only.
--
-- A human may disagree with the classifier, and the correction is an EVENT
-- rather than an edit: a rate that moved because a rep re-judged twenty replies
-- is a different fact from one that moved because customers changed their
-- minds, and an overwriting column cannot tell those apart afterwards. The
-- current verdict stays on the activity so every read stays one row; this table
-- is how it got there.
--
-- NO FREE-TEXT NOTE, and the omission is deliberate. Erasure UPDATES activity in
-- place — it never deletes the row — so the ON DELETE CASCADE below never fires
-- for an Art. 17 request, and anything a human typed here would outlive the
-- message it is about. Every column that remains is a closed vocabulary word, an
-- actor id or a timestamp: a reading OF the text rather than the text, which is
-- the same ground on which activity.owed_verdict and capture_label sit in the
-- erasure baseline. A corrector who needs to say why has the activity's own
-- surfaces for it, where erasure can reach the words.
CREATE TABLE IF NOT EXISTS activity_reply_verdict_history (
    id uuid PRIMARY KEY DEFAULT uuidv7(),
    activity_id uuid NOT NULL REFERENCES activity(id) ON DELETE CASCADE,
    -- NULL verdict records a correction back to unjudged, which a human may
    -- legitimately make: "nobody can tell from this message" is an answer.
    verdict text,
    -- Who said so: the classifier version, or the actor for a human correction.
    decided_by text NOT NULL,
    -- Whether a person or the model made this entry. A rate's provenance is a
    -- question people ask later, and deriving it from decided_by's spelling
    -- would make a naming change silently reclassify history.
    is_human boolean NOT NULL,
    decided_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT activity_reply_verdict_history_verdict
        CHECK (verdict IS NULL OR verdict IN ('positive', 'negative', 'neutral'))
);

CREATE INDEX IF NOT EXISTS idx_activity_reply_verdict_history_activity
    ON activity_reply_verdict_history (activity_id, decided_at DESC);

COMMENT ON TABLE activity_reply_verdict_history IS
    'Append-only record of how a reply verdict came to be what it is: the classifier''s own judgement and every human correction after it.';

-- The classifier's reply backlog: inbound correspondence the classify pass has
-- not yet judged.
--
-- Partial, so it indexes the work rather than the archive — the shape
-- idx_activity_unjudged and idx_activity_unlabeled both use. The audience and
-- hold clauses are here for the reason they are there: a message the worklist
-- may not open is not one to spend a model call on, and a row under a statutory
-- hold is out of reach of every ordinary read path.
CREATE INDEX IF NOT EXISTS idx_activity_unjudged_reply
    ON activity (occurred_at)
    WHERE reply_verdict IS NULL
      AND direction = 'inbound'
      AND kind IN ('email', 'message')
      AND archived_at IS NULL
      AND audience = 'workspace'
      AND restricted_at IS NULL;
