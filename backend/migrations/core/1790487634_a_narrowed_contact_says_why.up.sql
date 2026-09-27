-- A contact kept visible to its owner alone now records WHY.
--
-- Four decisions narrow a contact, and only one of them should end when the
-- address answers: mail we sent that nobody has replied to yet. A confidentiality
-- hold, an advisor verdict and a human's own "make private" must survive a reply.
-- Visibility alone cannot tell them apart, so a widening could not ask whether
-- THIS narrowing is one it may lift. The reason is written by the decision that
-- narrowed the row, because re-deriving it later reads present state and gets a
-- lifted hold wrong.
--
-- awaiting_verdict is the capture sink's own mint: nothing has judged the
-- sender yet, and a verdict is what ends it. NULL on an owner-scoped row is a
-- row narrowed before this column existed, for a reason nobody recorded, and no
-- automatic widening ends it.
--
-- Both constraints are added NOT VALID; the next migration validates them
-- under the lighter lock.
SET LOCAL lock_timeout = '3s';

ALTER TABLE contact
    ADD COLUMN IF NOT EXISTS narrowing_reason text,
    ADD CONSTRAINT contact_narrowing_reason_check CHECK (narrowing_reason IN (
        'awaiting_verdict', 'outbound_no_answer', 'confidentiality_hold', 'advisor', 'human_decided'))
        NOT VALID,
    -- A reason describes a row that is narrowed. Every widening clears it, and a
    -- widening that forgets to fails here rather than leaving a stale reason for
    -- the next narrowing to be read by.
    ADD CONSTRAINT contact_narrowing_reason_only_when_owner CHECK (
        narrowing_reason IS NULL OR visibility = 'owner') NOT VALID;

COMMENT ON COLUMN contact.narrowing_reason IS
    'Why an owner-scoped contact is its owner''s alone. A verdict widens awaiting_verdict and outbound_no_answer, a reply only outbound_no_answer; NULL is an unrecorded legacy narrowing and widens on neither.';
