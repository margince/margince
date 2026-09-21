-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion

-- Storing a file is two writes to two systems that cannot be made atomic: the
-- bytes go to the object store, then a row records where they are. Both writers
-- put the bytes FIRST, on the deliberate ground that an unreferenced object is
-- a better failure than a row promising bytes that are not there.
--
-- When the transaction after the put fails, the object stays. Nothing
-- references it, and nothing could ever delete it — an Art. 17 erasure finds an
-- attachment's bytes by reading storage_key off the attachment row, so an
-- object with no row is one the erasure cannot reach or even enumerate by
-- owner. It takes a transaction failure after a successful put to make one, so
-- they are rare; "rare and permanently unreachable" is still the wrong resting
-- state for a data subject's file.
--
-- THIS IS AN INTENT LEDGER, NOT A SWEEP. The alternative was to list the object
-- store and delete whatever the attachment table does not reference, and it was
-- refused: that is a bulk deleter over customer files whose correctness rests
-- on a diff, and one false negative in the diff destroys live documents. A
-- reaper over this table can only ever delete an object something told it was
-- provisional, so the same bug reaches nothing.
--
-- The row is written before the put and deleted in the SAME transaction that
-- writes the attachment row, so a key is provisional exactly while the pair is
-- incomplete. A key still here after the grace period is an object whose row
-- never arrived.
CREATE TABLE stored_object_intent (
    -- The object store key, which is what a reaper needs and all it needs. No
    -- workspace column: the key already carries the workspace prefix
    -- (blobstore.WorkspaceKey), and a second copy of the tenant would be a
    -- second answer to which prefix this object lives under.
    storage_key text NOT NULL,
    recorded_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT stored_object_intent_pkey PRIMARY KEY (storage_key)
);

-- The reaper's read: the oldest provisional keys first, so a pass that is
-- capped still makes progress on the ones that have waited longest.
CREATE INDEX stored_object_intent_recorded_idx ON stored_object_intent (recorded_at);
