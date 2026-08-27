-- ADR-0091 §1: the overlay mode moves off the workspace row.
--
-- x_sor_mode and x_incumbent are the last two columns on `workspace` that are
-- not identity or lifecycle, and they are the last thing standing between that
-- table and retirement. They are also the reason overlay holds a standing
-- waiver to write a table identity owns (backend/gates/tableownership_test.go): the
-- x_overlay_iff_incumbent CHECK requires both to move together with the
-- connection row, so routing them through identity would split one
-- transaction across a sibling module. Owning the row removes the waiver
-- rather than re-justifying it.
--
-- It is a table of its own rather than two `setting` rows because the CHECK is
-- the point. `(sor_mode = 'overlay') = (incumbent IS NOT NULL)` is a rule the
-- database has been holding since the columns were added; two independent
-- settings keys cannot express it, and the first write that set one without
-- the other would be the bug it exists to refuse.
--
-- The mode is NOT derivable from incumbent_connection, which is why this is a
-- move and not a deletion. A completed flip leaves the connection ACTIVE and
-- the mode NATIVE — CompleteFlip writes the mode and does not touch the
-- connection — so three states are reachable: never connected, connected in
-- overlay mode, and flipped to native with the connection still live.
-- Bounded before anything touches `workspace`, then held for the whole
-- migration.
--
-- The bound alone is not enough, and the gap it leaves is a lost write rather
-- than a stall: this transaction READS x_sor_mode, and only later does the
-- ALTER take its own ACCESS EXCLUSIVE. In between, a request served by a build
-- that still writes those columns can flip the mode and COMMIT — the ALTER
-- waits for it, then drops the column it just wrote, and overlay_mode carries
-- the value from before. The installation would come out of the migration
-- routing reads at a store its live connection disagrees with, with nothing to
-- report it.
--
-- Taking the lock up front closes that: no other transaction can write
-- `workspace` between the read and the drop, because none can touch it at all.
SET LOCAL lock_timeout = '3s';
LOCK TABLE workspace IN ACCESS EXCLUSIVE MODE;

CREATE TABLE overlay_mode (
    -- The singleton shape core uses for embed_store_binding, not the
    -- `UNIQUE (btree((true)))` its neighbours in this pack use, because that
    -- one needs a column to hang the index off: incumbent_connection already
    -- carries a uuid id, and this table would have to invent one nothing
    -- references. A boolean that can only be true is the whole key.
    --
    -- Either shape gives AT MOST one row. The other half — that a row EXISTS —
    -- is what the readers depend on, and it is held by the delete guard below
    -- rather than by the three exclusion lists that spare this table from the
    -- sweeps. Those lists are an optimisation; the guard is the rule.
    singleton  boolean     NOT NULL DEFAULT true,
    sor_mode   text        NOT NULL DEFAULT 'native',
    incumbent  text,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT overlay_mode_pkey PRIMARY KEY (singleton),
    CONSTRAINT overlay_mode_singleton_check CHECK (singleton),
    CONSTRAINT overlay_mode_sor_mode_check CHECK (sor_mode IN ('native', 'overlay')),
    CONSTRAINT overlay_mode_incumbent_check
        CHECK (incumbent IS NULL OR incumbent IN ('hubspot', 'salesforce', 'dynamics')),
    CONSTRAINT overlay_mode_overlay_iff_incumbent
        CHECK ((sor_mode = 'overlay') = (incumbent IS NOT NULL))
);

CREATE TRIGGER trg_overlay_mode_updated
    BEFORE UPDATE ON overlay_mode
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- The row is not deletable. Every reader of the mode scans this one row, and a
-- missing row is not a state any of them can answer: the dispatcher would have
-- no system of record to choose, and the fleet walkers would read "no
-- installation is in overlay mode" from a database that has simply lost the
-- fact. Before this guard, three separate exclusion lists — the production
-- sweep's preservedResetTables, testdb's resetTables, and the teardown suite's
-- retainedByDesign — were the only thing standing between a DELETE and that
-- state, and a fourth sweep site would have had to find all three.
--
-- Same posture as the append-only ledgers (audit_log_immutable), for the same
-- reason: a rule the database refuses beats a rule three lists remember.
CREATE FUNCTION overlay_mode_undeletable() RETURNS trigger
    LANGUAGE plpgsql
    AS $fn$
BEGIN
  RAISE EXCEPTION 'overlay_mode holds the installation''s system-of-record mode and always has exactly one row; it is updated, never deleted'
    USING ERRCODE = 'check_violation';
END; $fn$;

CREATE TRIGGER trg_overlay_mode_undeletable
    BEFORE DELETE ON overlay_mode
    FOR EACH ROW EXECUTE FUNCTION overlay_mode_undeletable();

-- More than one live workspace has no single mode to carry, and the DROP three
-- statements below makes the loss unrecoverable. identity.InstallationWorkspace
-- already refuses to serve such a database (ErrMultipleWorkspaces), so this
-- refuses to migrate it rather than silently keeping the oldest row's mode and
-- discarding the rest.
DO $$
DECLARE live int;
BEGIN
    SELECT count(*) INTO live FROM workspace WHERE archived_at IS NULL;
    IF live > 1 THEN
        RAISE EXCEPTION 'overlay_mode: % live workspaces, so there is no single system-of-record mode to carry onto the installation row. Resolve to one live workspace first (or rebuild with make dev-fresh); this migration will not choose between them.', live;
    END IF;
END $$;

-- Carry the installation's current mode across, from the workspace the
-- resolver would name (identity.activeWorkspaces: live rows only, and the
-- guard above has established there is at most one). A database with no live
-- workspace — a fresh one, mid-bootstrap — takes the column default instead,
-- which is the same 'native' a fresh workspace row would have carried.
INSERT INTO overlay_mode (sor_mode, incumbent)
SELECT x_sor_mode, x_incumbent
  FROM workspace
 WHERE archived_at IS NULL
 ORDER BY created_at, id
 LIMIT 1;

INSERT INTO overlay_mode (sor_mode, incumbent)
SELECT 'native', NULL
 WHERE NOT EXISTS (SELECT 1 FROM overlay_mode);

-- The three CHECK constraints go with the columns: each references only the two
-- being dropped, x_overlay_iff_incumbent included.
ALTER TABLE workspace
    DROP COLUMN x_sor_mode,
    DROP COLUMN x_incumbent;
