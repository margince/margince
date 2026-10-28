-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Whether each open deal is still supported by its customer, as the last
-- input check judged it, and the review episode a finding about it belongs to.
--
-- A receipt for EVERY deal the check assessed, including the eligible ones. A
-- missing finding cannot prove a deal was judged: a deal created after the check
-- ran has no finding either. The qualified forecast counts a deal only on a
-- current receipt.
--
-- New table and nullable or defaulted columns only, so no row is rewritten.
SET LOCAL lock_timeout = '3s';

CREATE TABLE assurance_deal_receipt (
    deal_id uuid PRIMARY KEY REFERENCES deal(id) ON DELETE CASCADE,
    run_id uuid NOT NULL REFERENCES assurance_run(id) ON DELETE CASCADE,
    outcome text NOT NULL
        CONSTRAINT assurance_deal_receipt_outcome_check
        CHECK (outcome IN ('eligible', 'review_required', 'waiting', 'unknown')),
    -- Reason codes, never prose: the reader's own language renders them.
    reasons text[] NOT NULL DEFAULT '{}',
    silent_days integer NOT NULL,
    policy_version text NOT NULL,
    -- What the assessment read about the deal. A reader whose current
    -- fingerprint differs knows the evidence moved since, and treats the
    -- receipt as unknown rather than trusting yesterday's answer.
    input_fingerprint text NOT NULL,
    assessed_at timestamptz NOT NULL,
    -- The first moment the answer can change by the clock alone, or the end of
    -- the check's cadence, whichever is sooner.
    valid_until timestamptz NOT NULL,
    captured_by text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT assurance_deal_receipt_valid_after_assessed CHECK (valid_until >= assessed_at)
);

CREATE INDEX assurance_deal_receipt_by_run ON assurance_deal_receipt (run_id);

-- The episode counts how often a finding came back after it was closed. The
-- task link names the one task that episode's review was delivered as, so a
-- completed task is not delivered a second time.
ALTER TABLE assurance_exception
    ADD COLUMN episode integer NOT NULL DEFAULT 1,
    ADD COLUMN task_activity_id uuid REFERENCES activity(id) ON DELETE SET NULL;
