-- Which rules judged an inbound message, recorded on the message.
--
-- owed_verdict was written once and never revisited: the CAS carried
-- `owed_verdict IS NULL`, so a second pass reported itself unapplied and the
-- first answer stood forever. That is right while the rules do not move. It is
-- wrong the moment they do — a prompt corrected to read "Dienstag 14 Uhr würde
-- bei uns passen" as a request cannot reach the messages the old prompt already
-- judged, and a customer waiting on an answer stays invisible for good.
--
-- The stamp is what makes "judged under which rules" answerable, and therefore
-- what makes a re-judge decidable rather than a second opinion. It holds a
-- digest of the classifier prompt (ai.PromptDigest, `prompts-<hex>`), so it
-- moves when the wording moves and stays put otherwise.
--
-- NULL WITH A VERDICT is a real state and not a gap: a judgement made before
-- this column existed. It is stale by definition, which is exactly what the
-- sweep needs, so nothing backfills it — an UPDATE over every judged row of
-- this table is the lock cost 1788489200 spends a paragraph avoiding, and the
-- sweep drains the population within hours anyway.
SET LOCAL lock_timeout = '3s';

ALTER TABLE activity
    ADD COLUMN IF NOT EXISTS owed_verdict_ruleset text;

-- One direction only. A ruleset naming a judgement that was never made is
-- nonsense and is refused; a verdict carrying no ruleset is the legacy row
-- above and is admitted. The consent decisions' own stamp reads the same way,
-- where both-NULL is an answer rather than a hole.
--
-- NOT VALID here, VALIDATE in the migration after this one. dbmigrate runs a
-- file in ONE transaction, so validating in this file would hold the ALTER's
-- ACCESS EXCLUSIVE for the whole scan and buy nothing — two files are two
-- transactions, which is the only shape where the split pays.
ALTER TABLE activity
    ADD CONSTRAINT activity_owed_verdict_ruleset_shape
    CHECK (owed_verdict_ruleset IS NULL OR owed_verdict IS NOT NULL)
    NOT VALID;

COMMENT ON COLUMN activity.owed_verdict_ruleset IS
    'Digest of the classifier prompt that produced owed_verdict. NULL beside a verdict is a judgement made before rulesets were recorded, and is stale by definition.';

-- The re-judge sweep's backlog: judged inbound correspondence, by the ruleset
-- that judged it.
--
-- Partial on JUDGED rows, mirroring idx_activity_unjudged's predicate for the
-- other half of the population. The two are disjoint by construction, so
-- neither has to serve the other's question and neither becomes an index over
-- the whole archive — which is what one index spanning both would be, growing
-- forever as judged rows accumulate and unjudged ones drain.
CREATE INDEX IF NOT EXISTS idx_activity_owed_ruleset
    ON activity (owed_verdict_ruleset, occurred_at)
    WHERE owed_verdict IS NOT NULL
      AND direction = 'inbound'
      AND kind IN ('email', 'message')
      AND archived_at IS NULL
      AND audience = 'workspace'
      AND restricted_at IS NULL;
