-- The six answers a person can give a finding, plus the one the system gives
-- itself.
--
-- The names shipped with the table were the module's own vocabulary; these are
-- the ones the review surface and the tool speak. Aligning them now, before any
-- row exists, costs a CHECK swap; aligning them later costs a data migration
-- and a period where two vocabularies mean the same thing.
--
-- `condition_cleared` is the system's, not a person's: a finding whose
-- condition stopped being true resolves itself, and the task it minted has to
-- close or it hangs in a seller's list forever.
SET LOCAL lock_timeout = '5s';

ALTER TABLE assurance_resolution
    DROP CONSTRAINT assurance_resolution_outcome_check;

ALTER TABLE assurance_resolution
    ADD CONSTRAINT assurance_resolution_outcome_check CHECK (outcome IN (
        -- The record was wrong and somebody corrected it.
        'fixed_record',
        -- The record was right and the evidence for it was missing.
        'added_evidence',
        -- The value is correct as it stands. SUPPRESSING: it hides the finding
        -- from every surface, which is why its expiry is capped.
        'value_correct',
        -- The finding does not apply to this deal. Also suppressing.
        'not_relevant',
        -- Not now. It comes back.
        'remind_later',
        -- Somebody else's to answer.
        'reassign',
        -- The system's own: the condition stopped being true.
        'condition_cleared'));

-- A suppressing answer names when it stops holding.
--
-- `value_correct` and `not_relevant` hide a finding from the screens a revenue
-- commitment is made from. Without an expiry that suppression is permanent, and
-- a value that was correct in May is a claim about May.
ALTER TABLE assurance_resolution
    ADD CONSTRAINT assurance_resolution_suppression_expires CHECK (
        outcome NOT IN ('value_correct', 'not_relevant') OR expires_at IS NOT NULL);

-- A deferral names when it comes back, or it is a dismissal wearing a
-- different word.
ALTER TABLE assurance_resolution
    ADD CONSTRAINT assurance_resolution_deferral_returns CHECK (
        outcome <> 'remind_later' OR remind_at IS NOT NULL);
