-- What a data provider says about a contact now fills the empty fields on
-- their record, instead of living only beside it.
--
-- Three things this needs, and one it deliberately does not.
--
-- The two CHECK widenings admit a vocabulary the code grew: a catch-up sweep
-- that reaches contacts older than the connection (`automatic_backfill`), and
-- the refusal it owes a contact the provider has nothing to match on
-- (`no_identifiers`) — declining before spending a call that can only answer
-- "no match".
--
-- `applied_at` is the marker that a run's answers reached the record. It is
-- distinct from `completed_at`: a run can be paid and complete while its
-- values are still absent from the record, and a client that stopped watching
-- at "completed" would show a page that still looks empty.
--
-- Every existing row is left NULL ON PURPOSE. Backfilling it from
-- `completed_at` would read as "already applied" for every run bought before
-- this change, and those are exactly the ones whose answers have never touched
-- a record — the sweep would then skip the contacts that most need it.
--
-- What it does NOT need is a value copy. provider_applied_field records WHICH
-- field a run filled, not what it said, except where nothing else can identify
-- the value later (see below).

SET LOCAL lock_timeout = '3s';

ALTER TABLE provider_run
  DROP CONSTRAINT provider_run_trigger_check;

ALTER TABLE provider_run
  ADD CONSTRAINT provider_run_trigger_check CHECK (trigger IN (
    'automatic_create', 'automatic_import', 'automatic_backfill',
    'scheduled_refresh', 'manual'));

ALTER TABLE provider_run
  DROP CONSTRAINT provider_run_skip_reason_check;

ALTER TABLE provider_run
  ADD CONSTRAINT provider_run_skip_reason_check CHECK (
    skip_reason IS NULL OR skip_reason IN (
      'budget_exhausted', 'low_balance', 'suppressed', 'not_eligible',
      'duplicate_subject_candidate', 'rate_limited', 'already_fresh',
      'no_identifiers'));

ALTER TABLE provider_run
  ADD COLUMN applied_at timestamptz;

-- Which record field each run filled, so the purchase can be taken back off
-- the record without touching what a colleague has since written there.
--
-- Two shapes in one table, because they answer the same question by different
-- means. A child row (an address, a number, an employment edge) is identified
-- by target_row_id, and that row carries the provider as its own source — its
-- continued existence IS the proof nobody replaced it, so no copy of the value
-- is kept. A plain column (person.title, a social handle) has no row to point
-- at and no source of its own, so applied_value holds what was written, as the
-- database stored it, and a revert clears the field only while it still says
-- that. A hash would answer the same question while being a reversible
-- fingerprint of the subject's own address or number; the value itself is no
-- worse and is honest about what it is.
--
-- Erasure and the retention sweep both delete these rows: they name a person
-- and describe what is on that person's record.
CREATE TABLE provider_applied_field (
    id uuid DEFAULT uuidv7() NOT NULL,
    person_id uuid NOT NULL,
    run_id uuid NOT NULL,
    provider text NOT NULL,
    target_table text NOT NULL,
    target_row_id uuid,
    target_field text NOT NULL,
    applied_value text,
    captured_by text NOT NULL,
    applied_at timestamptz DEFAULT now() NOT NULL,
    CONSTRAINT provider_applied_field_pkey PRIMARY KEY (id),
    CONSTRAINT provider_applied_field_person_id_fkey
      FOREIGN KEY (person_id) REFERENCES person(id) ON DELETE CASCADE,
    CONSTRAINT provider_applied_field_run_id_fkey
      FOREIGN KEY (run_id) REFERENCES provider_run(id) ON DELETE CASCADE,
    CONSTRAINT provider_applied_field_target_check CHECK (target_table IN (
      'person', 'person_social', 'person_email', 'person_phone', 'relationship')),
    -- A child row is identified by its id and keeps no value; a column keeps
    -- its value and has no row. Neither half is optional for its own kind, and
    -- carrying both would mean two answers to "is this still ours".
    CONSTRAINT provider_applied_field_identity_shape CHECK (
      (target_row_id IS NULL) <> (applied_value IS NULL))
);

CREATE UNIQUE INDEX provider_applied_field_once
  ON provider_applied_field (run_id, target_table, target_field, COALESCE(target_row_id, person_id));

CREATE INDEX provider_applied_field_by_person
  ON provider_applied_field (person_id, provider);

CREATE INDEX provider_applied_field_by_provider
  ON provider_applied_field (provider);

GRANT SELECT, INSERT, UPDATE, DELETE ON TABLE provider_applied_field TO margince_app;
