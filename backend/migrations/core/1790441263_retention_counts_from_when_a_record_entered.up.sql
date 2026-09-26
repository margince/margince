-- When a record entered THIS installation, separate from the date it carries.
--
-- Retention asks "has this been here longer than the policy allows", and it
-- answered from created_at (and a deal's closed_at). An import states those
-- dates from the source system — a lead first seen in 2023 keeps 2023 — so on
-- the first pass after an import the sweep acted on every record older than
-- the window, the day they arrived. On a rehearsal install 1,200 imported leads
-- were anonymized within ninety minutes.
--
-- entered_at is the clock the installation keeps for itself. It is set on
-- insert and nothing writes it afterwards: the API does not expose it, and no
-- production statement names it (TestNothingWritesWhenARecordEnteredTheInstall,
-- backend/gates/enteredatwriters_test.go). An import that backdates created_at
-- leaves it alone. Retention acts only once BOTH the record's own date and its
-- time in this installation are past the window.
--
-- Existing rows get the time of this migration, which delays their retention
-- rather than bringing it forward.
SET LOCAL lock_timeout = '3s';

ALTER TABLE lead ADD COLUMN entered_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE contact ADD COLUMN entered_at timestamptz NOT NULL DEFAULT now();
ALTER TABLE deal ADD COLUMN entered_at timestamptz NOT NULL DEFAULT now();

