-- Who reached a verdict, not only when.
--
-- The ledger recorded `resolved_at` and nothing about the authority behind it,
-- so a model's guess and an owner's explicit answer were indistinguishable
-- afterwards. That was survivable while a verdict only decided whether a record
-- was made. It stops being survivable when a verdict destroys mail: the purge
-- of `personal` correspondence needs to know whether a person actually said so.
--
-- Two authorities reach the same branch today — applyOwnerDecision, which is a
-- person clicking, and applyJudged, which is the classifier. This column is the
-- one place that difference survives the commit.
--
-- Backfilled FALSE, which is the honest answer rather than a convenient one:
-- every existing row was resolved before this column existed, and no evidence
-- remains that a human reached any of them. FALSE buys those rows the longer
-- window, which is the safe direction for a decision that destroys.
SET LOCAL lock_timeout = '3s';

-- NOT NULL with a DEFAULT rather than a nullable column: "we do not know"
-- and "the model decided" call for the same treatment here, and a third state
-- would only invite a reader to guess which of the two a NULL meant.
ALTER TABLE capture_pending_counterparty
    ADD COLUMN resolved_by_owner boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN capture_pending_counterparty.resolved_by_owner IS
    'Whether a person reached this verdict rather than the classifier. Gates how long the personal-mail purge waits before destroying.';
