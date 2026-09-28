SET LOCAL lock_timeout = '5s';

-- A decline names the prompt it answered. The *_declined_at stamps took a
-- message out of its sweep for good, which is right while the prompt stands
-- still and wrong the moment it moves: a reworded prompt may get an answer
-- from the same models, and a row declined under the old wording would never
-- be offered to it. Each stamp now carries the digest of the prompt that was
-- declined (ai.PromptDigest, `prompts-<hex>`), and a sweep re-offers a row
-- whose stamp names any other prompt.
--
-- NULL beside a stamp is a decline recorded before this column existed, and
-- reads as declined under another prompt: it is offered once more under the
-- current one and re-stamped if it is declined again. Nothing backfills it —
-- the population is the handful of rows every rung refused, and the sweeps
-- drain it. Nullable with no default, so adding them rewrites nothing.
ALTER TABLE activity
    ADD COLUMN capture_label_declined_ruleset text,
    ADD COLUMN owed_verdict_declined_ruleset text;

COMMENT ON COLUMN activity.capture_label_declined_ruleset IS
    'Digest of the capture-classify prompt every rung declined. A sweep under a different digest offers the message again.';
COMMENT ON COLUMN activity.owed_verdict_declined_ruleset IS
    'Digest of the owed-verdict prompt every rung declined. A sweep under a different digest offers the message again.';
