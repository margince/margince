-- A suppression carries the LEVEL of whoever decided it, so the product can say
-- who may lift it.
--
-- Until now every row looked alike: the engine's own reading, a rep's judgement
-- and the subject's own objection were four kinds and one undifferentiated
-- source string. That is enough to refuse a send and not enough to answer the
-- question a rep actually asks, which is "who said no, and can I overrule it".
--
-- The rule this column exists to serve: you may overrule a decision made BELOW
-- your level, never at or above it. machine < user < admin, and subject above
-- all of them — an Art. 21 objection to direct marketing is absolute, so no
-- seat in the installation lifts it, admin included.
--
-- source is NOT the place for this. It is free text describing WHERE a row came
-- from, and a permission rule keyed on a string people spell differently is a
-- rule that fails open the first time somebody writes 'Unsubscribe' instead of
-- 'unsubscribe'. A constrained column is the whole point.
-- The table is live: the engine reads it on every send. An unbounded ALTER
-- queues behind any open transaction and stalls every write for as long as it
-- is willing to wait, which is forever.
SET LOCAL lock_timeout = '3s';

-- The DEFAULT exists to fill EXISTING rows, and it is dropped again at the
-- bottom of this file. It is scaffolding, not the column's shape.
--
-- Leaving it in place would be the defect: a future bounce handler that
-- inserted a row without naming the column would get 'subject', which is the
-- one tier nothing can lift — so a customer who fixed a typo in their address
-- could never be written to again. The CASE below deliberately classifies a
-- hard bounce as 'machine' for exactly that reason, and a surviving DEFAULT
-- would silently overrule it for every row written from here on.
--
-- 'subject' while it IS in place, because it only ever touches rows that
-- predate the column: the safe reading of a row that does not say who decided
-- it is the one nobody can lift.
ALTER TABLE communication_suppression
    ADD COLUMN decided_by_level text NOT NULL DEFAULT 'subject';

-- Existing rows, classified by what they ARE rather than by what wrote them.
--
-- The safe direction is UP. Under-classifying a row makes it liftable by
-- somebody who should not be able to lift it, and the only irreversible mistake
-- here is lifting an objection that a person actually made. Over-classifying
-- costs somebody an escalation; under-classifying costs the subject their
-- objection, so every ambiguous case takes the stronger reading.
UPDATE communication_suppression
   SET decided_by_level = CASE
       -- The subject's own act, however it reached us. A marketing objection is
       -- Art. 21 by definition, and subject_request is the person asking in
       -- their own words. Both are the person the data is about.
       WHEN kind IN ('marketing_objection', 'subject_request') THEN 'subject'
       -- A statutory processing restriction is imposed on the controller and is
       -- not a seat's judgement to reverse. It sits with the subject's own tier
       -- because the same thing is true of it: nobody here may lift it.
       WHEN kind = 'processing_restriction' THEN 'subject'
       -- A hard bounce is the machine reporting a fact about the mailbox. It is
       -- the one kind a human may legitimately overrule — a corrected address,
       -- a mail server that has since been fixed.
       WHEN kind = 'hard_bounce' THEN 'machine'
       -- The safe answer to a kind this rule does not name is the tier nothing
       -- can lift. Deliberately not NULL: a row nobody classified must still
       -- refuse, and an installation whose data is fine must not fail to
       -- migrate because its vocabulary grew.
       ELSE 'subject'
   END;

ALTER TABLE communication_suppression
    ADD CONSTRAINT communication_suppression_decided_by_level
        CHECK (decided_by_level = ANY (ARRAY[
            'machine'::text,
            'user'::text,
            'admin'::text,
            'subject'::text
        ]));

-- Every existing row now names a level, so the scaffolding comes down. From
-- here a writer states who decided, or the INSERT fails where the author can
-- see it — which is the whole reason this column exists rather than a guess
-- derived from  at read time.
ALTER TABLE communication_suppression
    ALTER COLUMN decided_by_level DROP DEFAULT;
