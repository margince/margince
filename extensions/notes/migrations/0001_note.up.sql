-- SPDX-License-Identifier: BUSL-1.1
-- SPDX-FileCopyrightText: 2026 Gradion
--
-- notes's one table, and the file every next unit author copies — so it
-- says what is actually true of it in each of the two places it runs.
--
-- IN THE PRE-MERGE GATE (backend/tools/extmigrategate, `make
-- check-ext-migrations`) it is applied as a restricted ext_notes role minted
-- against a throwaway database, holding CREATE on the ext schema and nothing at
-- all on public. That is what keeps every line
-- below to the narrowest shape such a role can produce, and the gate re-reads
-- the resulting catalog to prove it.
--
-- AT RUNTIME there is NO ext_notes role. cmd/migrate opens ONE
-- margince_owner connection and issues no SET ROLE, so this table is created
-- and owned by margince_owner exactly as every core table is. Do not read the
-- gate's restriction as a production DDL boundary: what bounds this table in
-- production is the grant surface extmigrategate polices and the ext schema the
-- unit owns — not its ownership, and not a row-level policy (see below).
-- backend/migrations/core/0213_ext_schema
-- states the same thing at the schema, and issue #628 tracks minting the
-- per-unit runtime role that would make ownership mean something here.
--
-- The name is ext_notes_note, not note: the ext schema is shared by every
-- installed unit, so the unit namespace is what keeps two of them from
-- colliding or addressing each other's rows.
--
-- THE AUTHOR COLUMNS BELOW WERE ADDED TO THIS FILE IN PLACE, and that is an
-- EXCEPTION rather than the rule the sibling migrations state. 0002 and 0003
-- both say — correctly, and for every unit that has shipped — that an amended
-- 0001 never re-runs: dbmigrate keys on the version, so the change lands on
-- exactly the installations that did not need it and on none that do. That
-- reasoning holds; what makes it moot HERE is that this unit is still in heavy
-- development, backward compatibility is explicitly not required of it, and
-- every dev and UAT database carrying it is recreated rather than upgraded. A
-- unit that has been installed anywhere an operator would notice must take the
-- new number instead.

CREATE TABLE ext.ext_notes_note (
    id              uuid        NOT NULL DEFAULT gen_random_uuid() PRIMARY KEY,
    body            text        NOT NULL,
    -- WHO wrote it. Stamped by the handler from the invocation's Caller and
    -- never from the request body, because an author a client supplies is an
    -- author a client forges; the enforcing half of that is in note.go, this is
    -- only where it is kept.
    --
    -- NO FOREIGN KEY TO THE USER TABLE, and its absence is a property of the
    -- gate rather than an oversight. The ext_notes role this file is applied as
    -- holds NOTHING on public at all, so a
    -- reference to a core user table here does not fail at review — it fails
    -- when the pre-merge gate applies the file, for every unit that copies this
    -- template. The id is therefore a plain uuid and the join, when a reader
    -- wants a name, is the core's to make. The practical cost is real and worth
    -- stating: nothing deletes these rows when the user is deleted, so an
    -- author_user_id can outlive the account it names and a reader must treat
    -- it as an id that may no longer resolve.
    --
    -- NULLABLE, because the tick has no author. A scheduled job's Caller is the
    -- zero value (CallerSystem, empty UserID) — there is no person behind it,
    -- and writing the zero uuid or a synthetic id would make "nobody wrote
    -- this" indistinguishable from a row whose author the reader simply cannot
    -- resolve.
    author_user_id  uuid,
    author_is_agent boolean,
    created_at      timestamptz NOT NULL DEFAULT now(),
    -- Both or neither. Split across two nullable columns, "an agent acting for
    -- nobody" (is_agent set, user id null) and "a person whose agent-ness is
    -- unknown" (the reverse) are both representable and neither means anything
    -- — so the database refuses them rather than leaving every reader to decide
    -- what a half-written author is. This is also what keeps the handler's
    -- both-or-neither stamping honest: getting it wrong is an error at the
    -- INSERT, not a row nobody notices.
    CONSTRAINT ext_notes_note_author_coherent
        CHECK ((author_user_id IS NULL) = (author_is_agent IS NULL))
);

-- NO ROW-LEVEL SECURITY, and for a unit table that is the rule rather than an
-- omission — state it, because this file is the template.
--
-- An installation holds exactly one active workspace
-- (identity.InstallationWorkspace refuses a second), so a tenant predicate
-- would name an isolation there is nothing left to isolate — and it would not
-- be a wall against a UNIT in any case, since a unit issues its own SQL through
-- the seam.
--
-- What bounds a unit is the grant below and the schema it owns. A row-level
-- rule can mean something here again once it keys on something a unit cannot
-- set, which is the per-unit role issue #628 tracks.

-- The app role runs the unit's handlers. TRUNCATE is deliberately absent: no
-- unit verb issues one, and a privilege nothing reaches for is one more thing
-- a compromised unit could.
--
-- Note what this grant is NOT, since every unit issues one: margince_app is the
-- SHARED runtime role, so this line gives it to every unit's handlers, not only
-- to notes's. Another installed unit's SQL can read and
-- write this table. That is inside the tier's trusted-unit threat model (see
-- backend/pkg/extension/runtime.go) and it is what #628's per-unit role would
-- close; it is stated here because this file is the template.
GRANT SELECT, INSERT, UPDATE, DELETE ON ext.ext_notes_note TO margince_app;
