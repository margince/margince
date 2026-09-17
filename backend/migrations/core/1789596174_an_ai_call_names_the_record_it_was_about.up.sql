-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Bounded, because ALTER TABLE takes ACCESS EXCLUSIVE on a table this migration
-- did not create: a writer holding a row lock would otherwise queue every
-- reader behind this statement for as long as it cared to wait. Two seconds is
-- the tree's own number for an additive column.
SET LOCAL lock_timeout = '2s';

-- Which record a model call was ABOUT, so an erasure can find the payloads of
-- calls made against the subject rather than only the payloads that happen to
-- spell one of their addresses.
--
-- With payload capture enabled, ai_call_payload holds the request and response
-- of every call. For a transcript reading that request IS the meeting
-- transcript, and a meeting quotes people by NAME — so an Art. 17 erasure that
-- reaches this table only by matching the subject's email addresses leaves the
-- largest copy of their words standing, until the retention window ages it out
-- on its own schedule. There was no other route: audit_log carries no
-- correlation_id to join on, transcript_read names no call, and agent_run_id is
-- null for a worker job.
--
-- NULLABLE, and the nulls are the honest part. A call whose input spans several
-- records — an account brief, a site read over a company's pages — names the
-- SUBJECT of the call and not every record it touched, and where there is no
-- single subject it names none. A list that is right half the time is a
-- citation nobody can trust for a purge, so the answer to "several" is none.
--
-- The citation is therefore an OPTIMISATION FOR THE REACHABLE HALF, never the
-- boundary: the content match stays exactly as it was, and a call that names no
-- record is reached by it or not at all. The erasure says so where it runs.
--
-- No backfill. Rows written before this migration were made by sites that kept
-- no answer, and a fabricated one would be worse than a null a reader can see.
ALTER TABLE ai_call
    ADD COLUMN subject_type text,
    ADD COLUMN subject_id uuid,
    -- Both or neither: one writer fills them from one value, and a row with
    -- half a citation is one the purge would either miss or mis-match.
    ADD CONSTRAINT ai_call_subject_shape CHECK ((subject_type IS NULL) = (subject_id IS NULL));

-- The purge's lookup, and the only read this column has. Partial, because the
-- rows worth finding are the ones that named something: on a telemetry table
-- most rows carry no citation, and an index over their nulls would be paid for
-- on every insert to answer nothing.
CREATE INDEX ai_call_subject_idx ON ai_call (subject_type, subject_id) WHERE subject_id IS NOT NULL;
