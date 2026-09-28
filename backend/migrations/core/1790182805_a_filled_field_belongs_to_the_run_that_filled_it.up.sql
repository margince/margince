SET LOCAL lock_timeout = '5s';

-- A provider_applied_field row belongs to the run it names.
--
-- The table carries contact_id, provider and run_id as independent columns,
-- and nothing required the first two to agree with the provider_run the third
-- points at. A row could claim that run R — about contact A, from provider X —
-- filled a field on contact B from provider Y, and only the writer's own
-- correctness stopped it.
--
-- That matters because a REVERT reads these rows to decide what to clear. A
-- row whose contact does not match its run clears a field on the wrong
-- contact, or clears one twice: the unique index keys on
-- (run_id, target_table, target_field, COALESCE(target_row_id, contact_id)),
-- so for a scalar marker the contact id is what separates rows, and one run
-- naming two contacts carries two markers for one field.
--
-- The UNIQUE below is what an FK can reference, and it is NOT the redundant
-- shape a primary key already covers: `id` alone is unique, so this index adds
-- no uniqueness — it exists so the triple is referenceable, which the primary
-- key cannot do. That is a purpose the second index on `lead(id)` removed in
-- 1790143300 did not have.
ALTER TABLE provider_run
    ADD CONSTRAINT provider_run_subject_identity UNIQUE (id, contact_id, provider);

-- NOT VALID, deliberately and permanently until somebody measures otherwise.
--
-- It binds every INSERT and UPDATE from here on, which is the whole guarantee
-- being asked for. What it does not do is scan history, and history is where
-- this cannot be checked from a migration: an installation that erased a
-- subject before the two erasure paths ordered themselves correctly may hold a
-- marker whose run was scrubbed out from under it, and a VALIDATE would fail
-- the migration on exactly the installations that have been running longest.
--
-- The ordering the constraint now depends on is already what both erasure
-- paths do: purgeProviderPurchases and purgeSubjectPurchases each DELETE the
-- markers and only then scrub the run (contact_id = NULL, subject_kind =
-- 'scrubbed'). A scrubbed run can never match a marker — the marker's
-- contact_id is NOT NULL — so the delete has to come first, and now it has to
-- stay first.
-- DEFERRED, because a merge legitimately moves both sides of this key.
--
-- relinkProviderPurchases carries a merged-away record's purchases to the
-- survivor: the markers move, then three statements decide which of two
-- colliding runs keeps its reservation — each selecting the SOURCE's runs by
-- contact_id — and only then do the runs move. Between those two moves the
-- markers name the survivor and their runs still name the source, which an
-- immediate check refuses mid-transaction.
--
-- Reordering is not the fix: the three statements in between are written
-- against contact_id = source, and moving the runs first makes every one of
-- them select nothing. The guarantee wanted here is about what is TRUE when
-- the transaction commits, not about what is true between two of its
-- statements, so that is what the constraint says. A transaction that ends
-- with a marker naming a run about somebody else still fails, and fails whole.
ALTER TABLE provider_applied_field
    ADD CONSTRAINT provider_applied_field_run_subject_fkey
    FOREIGN KEY (run_id, contact_id, provider)
    REFERENCES provider_run (id, contact_id, provider) ON DELETE CASCADE
    DEFERRABLE INITIALLY DEFERRED
    NOT VALID;
