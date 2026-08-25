-- ADR-0091 §1: the overlay mode moves off the workspace row.
--
-- x_sor_mode and x_incumbent are the last two columns on `workspace` that are
-- not identity or lifecycle, and they are the last thing standing between that
-- table and retirement. They are also the reason overlay holds a standing
-- waiver to write a table identity owns (backend/tableownership_test.go): the
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
CREATE TABLE overlay_mode (
    -- The singleton shape core uses for embed_store_binding, not the
    -- `UNIQUE (btree((true)))` its neighbour incumbent_connection uses. That
    -- one admits zero rows, which is right for a connection that may not
    -- exist; the mode always exists, so a primary key over a column that can
    -- only be true says exactly one row and says it structurally.
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

-- Carry the installation's current mode across, from the workspace the
-- resolver would name (identity.activeWorkspaces: live rows only, and this
-- installation has exactly one). A database with no live workspace — a fresh
-- one, mid-bootstrap — takes the column default instead, which is the same
-- 'native' a fresh workspace row would have carried.
INSERT INTO overlay_mode (sor_mode, incumbent)
SELECT x_sor_mode, x_incumbent
  FROM workspace
 WHERE archived_at IS NULL
 ORDER BY created_at, id
 LIMIT 1;

INSERT INTO overlay_mode (sor_mode, incumbent)
SELECT 'native', NULL
 WHERE NOT EXISTS (SELECT 1 FROM overlay_mode);

-- Bounded, because DROP COLUMN takes ACCESS EXCLUSIVE on a table this
-- migration did not create. The three CHECK constraints go with the columns:
-- each references only the two being dropped, x_overlay_iff_incumbent
-- included.
SET LOCAL lock_timeout = '3s';

ALTER TABLE workspace
    DROP COLUMN x_sor_mode,
    DROP COLUMN x_incumbent;
