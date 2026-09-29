-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- The evidence a restore drill leaves behind.
--
-- The rule, written out rather than cited: the installation undertakes to lose
-- at most an hour of data and to be back within four hours, and to rehearse
-- the restore and keep the evidence that it did.
--
-- Until now nothing recorded whether either number was ever true. A drill that runs and is not written down proves nothing
-- afterwards, which is exactly when somebody asks.
--
-- Both published numbers are DERIVED from this row rather than stored beside
-- it. The recovery window is finished_at - started_at; the data-loss window is
-- started_at - restored_to. Storing either as its own column would let it
-- disagree with the timestamps it came from, and a number that disagrees with
-- its own evidence is worse than no number.
--
-- Installation-level, with no workspace column: one installation serves one
-- company (ADR-0061) and a restore takes the whole database, not one tenant's
-- share of it.
SET LOCAL lock_timeout = '3s';

CREATE TABLE restore_drill (
    id             uuid PRIMARY KEY DEFAULT uuidv7(),
    -- When the drill began and ended. finished_at is NULL while it runs, which
    -- is also how an abandoned drill reads: started, never finished, and
    -- therefore never evidence of anything.
    started_at     timestamptz NOT NULL DEFAULT now(),
    finished_at    timestamptz,
    -- The point in time the backup was restored TO. What makes the data-loss
    -- window measurable rather than asserted.
    restored_to    timestamptz NOT NULL,
    -- passed: the restored installation was verified. failed: it was not, and
    -- that is evidence too — a drill that only recorded its successes would be
    -- a record of nothing.
    outcome        text NOT NULL DEFAULT 'running'
                   CHECK (outcome IN ('running', 'passed', 'failed')),
    -- Who ran it, as the actor string the audit spine uses.
    operator       text NOT NULL,
    -- What was checked, or why it failed.
    notes          text,
    created_at     timestamptz NOT NULL DEFAULT now(),

    -- A finished drill has an outcome, and a running one has not finished.
    CONSTRAINT restore_drill_outcome_follows_completion
        CHECK ((finished_at IS NULL) = (outcome = 'running')),
    -- Recovery cannot end before it began, and cannot restore to a point after
    -- it started: both would make the derived windows negative.
    CONSTRAINT restore_drill_finishes_after_it_starts
        CHECK (finished_at IS NULL OR finished_at >= started_at),
    CONSTRAINT restore_drill_restores_to_the_past
        CHECK (restored_to <= started_at)
);

-- The one question the operator surface asks: what is the most recent drill?
CREATE INDEX idx_restore_drill_recent ON restore_drill (started_at DESC);

-- The app role writes and reads it; without this the ledger is owner-only and
-- every drill recorded through the running server fails on permission.
GRANT SELECT, INSERT, UPDATE ON restore_drill TO margince_app;
