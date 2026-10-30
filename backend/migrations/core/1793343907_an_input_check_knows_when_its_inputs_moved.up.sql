-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Which inputs of the last input check changed after it read them.
--
-- A deal edited at noon still shows last night's findings until the next pass.
-- The findings are not wrong to keep, but a reader must see that the record has
-- moved since, or they act on a reading the record already left behind.
--
-- New table, new sequence and a nullable column, so no row is rewritten. The
-- unused assurance_exception.input_changed_at goes: it could mark only a deal
-- that already had a finding, never the run as a whole or a deal it had judged
-- fine, and nothing ever wrote it.
SET LOCAL lock_timeout = '3s';

-- One counter for every change, never a clock. A run records the highest value
-- it could see before it read the deals, and a change is unread when its value
-- is higher. Clocks of two processes can disagree; this cannot.
CREATE SEQUENCE assurance_input_generation_seq;

-- The default privileges cover tables only, so the app role needs USAGE to call
-- nextval(). Conditional because a throwaway test database has no app role.
DO $$
BEGIN
  IF EXISTS (SELECT FROM pg_roles WHERE rolname = 'margince_app') THEN
    GRANT USAGE, SELECT ON SEQUENCE assurance_input_generation_seq TO margince_app;
  END IF;
END $$;

-- One row per subject whose inputs moved, so a burst of edits to one deal is
-- one row however long it runs. Derived state with no audit or outbox of its
-- own: the domain write that caused each change carries the write shape.
--
-- subject_id names a deal or a signal the way assurance_exception.subject_id
-- does, with no foreign key: a deleted subject leaves a row nobody joins to. A
-- change that names no subject (a stage, a pipeline, a forecast call) is the one
-- 'workspace' row, keyed by the nil id.
CREATE TABLE assurance_input_change (
    subject_kind text NOT NULL
        CONSTRAINT assurance_input_change_subject_kind_check
        CHECK (subject_kind IN ('deal', 'signal', 'workspace')),
    subject_id uuid NOT NULL,
    generation bigint NOT NULL,
    changed_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT assurance_input_change_pkey PRIMARY KEY (subject_kind, subject_id)
);

CREATE INDEX assurance_input_change_by_generation ON assurance_input_change (generation);

-- The highest change this run could see before it read the deals. NULL on a
-- run that did not read all it needs, and on every run from before this column:
-- neither may clear a change. While no run has one, freshness is unknown.
ALTER TABLE assurance_run ADD COLUMN read_through_generation bigint;

ALTER TABLE assurance_exception DROP COLUMN input_changed_at;
