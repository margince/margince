-- Bound the wait for the locks below. Without it an open transaction holding a
-- conflicting lock stalls every write to consent_event for as long as this
-- migration is willing to queue, which is forever.
SET LOCAL lock_timeout = '3s';

-- policy_text records WHAT THE SUBJECT WAS SHOWN, and only a grant has one.
--
-- The column has been NOT NULL since the baseline, and the writer satisfied it
-- by coalescing an absent wording to the literal 'recorded via API'. That made
-- every wordless grant produce a row reading like proof and holding none — the
-- subject access export returns that sentence as what the person was shown.
--
-- A grant now carries real wording or is refused (requireWordingForGrant), so
-- the placeholder is gone. What is left is the WITHDRAWAL: nothing is being
-- demonstrated when somebody takes consent back, there is no wording to record,
-- and a placeholder there would invent a claim about a screen that never
-- existed. NULL is the honest value, and the column has to admit it.
--
-- Not a widening of what a grant may store: a grant reaching the writer without
-- wording is refused before the INSERT, and consenteventwording_test.go holds
-- that. This only lets a withdrawal say nothing.
ALTER TABLE consent_event ALTER COLUMN policy_text DROP NOT NULL;

-- policy_version travels with the text for the same reason: a version naming
-- wording that does not exist is a number pointing at nothing.
ALTER TABLE consent_event ALTER COLUMN policy_version DROP NOT NULL;

-- Both or neither, and only on a grant. A row carrying a version with no text
-- cannot say what that version SAID, and one carrying text with no version
-- cannot be told apart from a later edit of the same wording.
-- The rows already carrying the placeholder are cleared, not left standing.
--
-- Dropping NOT NULL only stops NEW wordless rows from inventing a sentence;
-- every row the old writer produced still reads 'recorded via API', and the
-- subject access export goes on returning it as what that person was shown.
-- That is the defect this migration is named for, so it is fixed for the rows
-- that have it rather than only for rows nobody has written yet.
--
-- NULL is the honest replacement: the wording was never captured, and no value
-- available here can say what somebody read. The version goes with it, because
-- the CHECK below stores the two together.
-- Matched as a PAIR, not on the text alone. The old writer coalesced the two
-- columns together — 'recorded via API' always beside version 'v1' — so the
-- pair is the placeholder's signature. Matching the sentence by itself would
-- also clear a row where somebody genuinely typed those words under a different
-- version, and that is real evidence this migration must not destroy.
UPDATE consent_event
   SET policy_text = NULL, policy_version = NULL
 WHERE (policy_text = 'recorded via API' AND policy_version = 'v1')
    -- The down migration's own filler, so a down/up cycle returns to where it
    -- started instead of leaving fabricated pairs the up path does not know.
    OR (policy_text = 'no wording recorded' AND policy_version = 'v1');

ALTER TABLE consent_event
    ADD CONSTRAINT consent_event_wording_pairs
        CHECK ((policy_text IS NULL) = (policy_version IS NULL));
