// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package main

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/jackc/pgx/v5"
)

// appRole is the runtime role an extension table may be granted DML on. It is
// the only grantee outside the owner that the allowlist admits: the app reads
// and writes extension rows under RLS, and nothing else has business here.
const appRole = "margince_app"

// tableDML is the privilege set appRole may hold on a TABLE, as PostgreSQL
// spells it in an ACL item: a(ppend/INSERT), r(ead/SELECT), w(rite/UPDATE),
// d(elete). Notably absent are D (TRUNCATE — bypasses the policy's USING clause
// and empties every tenant's rows at once), x (REFERENCES) and t (TRIGGER).
const tableDML = "arwd"

// sequenceUse is the equivalent for a SEQUENCE: r(SELECT), w(UPDATE) and
// U(SAGE). USAGE is the one that matters — an INSERT into a table whose column
// defaults to nextval() needs it — and a sequence carries no tenant rows, so
// the allowlist here is about completeness rather than isolation.
const sequenceUse = "rwU"

// tenantColumn is the name a unit table must NOT carry. It named the workspace
// a row belonged to while the tier was multi-tenant; assertNoTenantColumn says
// why a column of that name is now refused rather than required.
const tenantColumn = "workspace_id"

// relation is one row of the ext schema's contents.
type relation struct {
	oid         uint32
	name        string
	kind        rune
	persistence rune
	owner       string
	rls, force  bool
}

// validateCatalog asserts that what the unit's migrations left in the database
// is exactly an allowlisted shape, and refuses everything else by name.
//
// It runs over the ext_<name> connection, not an admin one. Every fact below
// is a catalog read that any role may make, so the choice does not change what
// is visible — but the RLS probe at the end is only meaningful from a role that
// row-level security actually binds, and running the whole validation from one
// connection removes the chance of the probe drifting onto the exempt one.
func validateCatalog(ctx context.Context, conn *pgx.Conn, namespace string) error {
	if err := assertNoStrayObjects(ctx, conn, namespace); err != nil {
		return err
	}
	relations, err := relationsInExt(ctx, conn)
	if err != nil {
		return err
	}
	if len(relations) == 0 {
		return fmt.Errorf("the migrations created no relation in schema %s — a unit with migrations owns at least one table there", extSchema)
	}
	prefix := namespace + "_"
	for _, rel := range relations {
		if err := assertRelationAllowed(ctx, conn, rel, namespace, prefix); err != nil {
			return err
		}
	}
	if err := assertNoTriggers(ctx, conn); err != nil {
		return err
	}
	return assertNoRules(ctx, conn)
}

// assertRelationAllowed applies the per-relation rules: who owns it, whether
// its name is inside the unit's namespace, and whether its relkind is one an
// extension may create at all.
func assertRelationAllowed(ctx context.Context, conn *pgx.Conn, rel relation, namespace, prefix string) error {
	where := extSchema + "." + rel.name
	if rel.owner != namespace {
		return fmt.Errorf("%s is owned by %q, not by the unit's %s role — an object the unit does not own is one its own migrations cannot revert", where, rel.owner, namespace)
	}
	// Enforced over EVERY relkind, not only tables: indexes, sequences and
	// views share one per-schema relation namespace with tables in PostgreSQL,
	// so an index named ext_a_b_c collides with another unit's table of that
	// name. gen-composition's textual rule collects tables only and delegates
	// the rest here.
	if !strings.HasPrefix(rel.name, prefix) {
		return fmt.Errorf("%s is outside the unit's namespace — every relation a unit creates is %s<name>, which is what keeps two units sharing the %s schema from colliding or addressing each other's rows", where, prefix, extSchema)
	}

	switch rel.kind {
	case 'r', 'p':
		return assertUnitTable(ctx, conn, rel, namespace)
	case 'i', 'I':
		return nil // an index carries no rows of its own, and pg_class.relacl is always null for one
	case 'S':
		// A sequence holds no rows of its own, but it does carry an ACL, and
		// this allowlist advertises completeness.
		return assertGrants(ctx, conn, rel, namespace, where, sequenceUse, "")
	case 'f':
		return fmt.Errorf("%s is a FOREIGN TABLE — its rows live outside this database entirely, so nothing here bounds what a unit reads or writes through it; extension data lives in ordinary tables in %s", where, extSchema)
	case 'm':
		return fmt.Errorf("%s is a MATERIALIZED VIEW — it holds a standing copy of whatever it selected, which outlives the grants on the tables it read and is refreshed by nothing this gate can see", where)
	case 'v':
		return fmt.Errorf("%s is a VIEW — a view is not in the allowlist: it runs with its owner's rights over the base tables, so it reaches whatever its owner can and the grant surface below says nothing about it", where)
	default:
		return fmt.Errorf("%s has relkind %q, which is not in the allowlist (ordinary and partitioned tables, their indexes and sequences)", where, string(rel.kind))
	}
}

// assertUnitTable is the shape every extension table must have.
//
// It is written as a set of REFUSALS rather than requirements because that is
// what the tier's rules now are: a unit table owns its rows, names no tenant,
// points nowhere outside the ext schema, and carries no row-level rule. Each
// one below is a thing PostgreSQL would happily accept and this gate will not.
func assertUnitTable(ctx context.Context, conn *pgx.Conn, rel relation, namespace string) error {
	where := extSchema + "." + rel.name
	if rel.persistence != 'p' {
		return fmt.Errorf("%s has relpersistence %q — an UNLOGGED or TEMPORARY table loses its rows on a crash or at disconnect, silently", where, string(rel.persistence))
	}
	if err := assertNoTenantColumn(ctx, conn, rel, where); err != nil {
		return err
	}
	if err := assertNoForeignKeysOutOfExt(ctx, conn, rel, where); err != nil {
		return err
	}
	if err := assertNoRowLevelSecurity(ctx, conn, rel, where); err != nil {
		return err
	}
	return assertGrants(ctx, conn, rel, namespace, where, tableDML, tableDML)
}

// assertNoTenantColumn refuses a workspace column on a unit table.
//
// An installation holds one organization, so a workspace column separates
// nothing — and it was never a wall against a unit in any case, since a unit
// issues its own SQL through the seam.
//
// Refused rather than merely not required: a column re-added here would read as
// a live tenant boundary to the next author, and the thing that would restore
// one is a per-unit database role, not a column.
func assertNoTenantColumn(ctx context.Context, conn *pgx.Conn, rel relation, where string) error {
	var present bool
	if err := conn.QueryRow(ctx, `
		SELECT EXISTS (SELECT 1 FROM pg_attribute a
		                WHERE a.attrelid = $1 AND a.attname = $2
		                  AND a.attnum > 0 AND NOT a.attisdropped)`,
		rel.oid, tenantColumn).Scan(&present); err != nil {
		return fmt.Errorf("reading %s's columns: %w", where, err)
	}
	if present {
		return fmt.Errorf("%s carries a %s column — an installation holds one workspace, so the column separates nothing and a policy over it would read as a tenant boundary that is not there; per-unit isolation is a database ROLE, not a column", where, tenantColumn)
	}
	return nil
}

// assertNoForeignKeysOutOfExt refuses a foreign key from a unit table to
// anything outside the ext schema.
//
// Held by PRIVILEGE first — the unit's role has no grants on public at all, so
// PostgreSQL refuses such a key at CREATE time — and asserted here anyway,
// because the gate's role is what makes that true and a gate that trusts its
// own setup proves nothing about a schema built any other way. A key into core
// takes a lock on core writes and can refuse a core delete forever after, which
// is a unit reaching into the product's own operations.
func assertNoForeignKeysOutOfExt(ctx context.Context, conn *pgx.Conn, rel relation, where string) error {
	var name, parent string
	err := conn.QueryRow(ctx, `
		SELECT co.conname, n.nspname || '.' || parent.relname
		  FROM pg_constraint co
		  JOIN pg_class parent ON parent.oid = co.confrelid
		  JOIN pg_namespace n ON n.oid = parent.relnamespace
		 WHERE co.conrelid = $1 AND co.contype = 'f' AND n.nspname <> $2
		 ORDER BY co.conname LIMIT 1`, rel.oid, extSchema).Scan(&name, &parent)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return nil
	case err != nil:
		return fmt.Errorf("reading %s's foreign keys: %w", where, err)
	}
	return fmt.Errorf("%s declares foreign key %s onto %s — a unit's tables reference nothing outside %s, because a key into core locks core writes and can refuse a core delete", where, name, parent, extSchema)
}

// assertNoRowLevelSecurity refuses RLS and any policy on a unit table.
//
// Both halves, because they fail differently: relrowsecurity without a policy
// denies every row to a non-owner and reads at a glance like a table that is
// merely protected, and a policy without RLS enabled is dead text that the next
// author will cite as a boundary. See assertNoTenantColumn for why the tier
// carries neither.
func assertNoRowLevelSecurity(ctx context.Context, conn *pgx.Conn, rel relation, where string) error {
	if rel.rls || rel.force {
		return fmt.Errorf("%s has relrowsecurity=%t relforcerowsecurity=%t — a unit table carries no row-level rule; the tenant it would key on is gone and a per-unit ROLE is what would make one mean something", where, rel.rls, rel.force)
	}
	var policies int
	if err := conn.QueryRow(ctx,
		`SELECT count(*) FROM pg_policy WHERE polrelid = $1`, rel.oid).Scan(&policies); err != nil {
		return fmt.Errorf("reading %s's policies: %w", where, err)
	}
	if policies > 0 {
		return fmt.Errorf("%s carries %d policy/policies with row-level security disabled — dead text the next reader would cite as a boundary", where, policies)
	}
	return nil
}

// assertGrants refuses every privilege outside the allowlist, on the relation
// and on its individual columns. allowed is the privilege set appRole may hold
// on this relkind — tableDML for a table, sequenceUse for a sequence, which are
// different letters and must not be checked against one list.
//
// required is the set appRole must hold, and it is not the same question. The
// allowlist alone is a one-sided gate: a tenant table granted NOTHING to the
// runtime role, or granted only SELECT, satisfies "nothing outside the list"
// perfectly and then answers `permission denied` at the first handler call —
// exactly the class of defect this gate exists to catch before a deployment
// does. An empty required skips the check, which is the sequence case: whether
// a unit's sequence needs the runtime role at all depends on whether a column
// defaults from it, and demanding the grant unconditionally would refuse an
// internally-used sequence that is correct.
func assertGrants(ctx context.Context, conn *pgx.Conn, rel relation, namespace, where, allowed, required string) error {
	var acl []string
	if err := conn.QueryRow(ctx,
		`SELECT coalesce(c.relacl::text[], '{}') FROM pg_class c WHERE c.oid = $1`, rel.oid,
	).Scan(&acl); err != nil {
		return fmt.Errorf("reading %s's grants: %w", where, err)
	}
	var held string
	for _, item := range acl {
		grantee, privileges, ok := strings.Cut(item, "=")
		if !ok {
			return fmt.Errorf("%s carries an unreadable ACL item %q", where, item)
		}
		privileges, _, _ = strings.Cut(privileges, "/")
		switch {
		case grantee == namespace:
			continue // the owner's own entry
		case grantee == "":
			return fmt.Errorf("%s grants %q to PUBLIC — every role on the cluster, including ones that predate this unit and ones this installation never provisioned", where, privileges)
		case grantee != appRole:
			return fmt.Errorf("%s grants %q to %q — only the owner and %s may hold privileges on an extension table", where, privileges, grantee, appRole)
		case strings.Trim(privileges, allowed) != "":
			return fmt.Errorf("%s grants %q to %s — only %q is allowed on this relation; TRUNCATE in particular empties the whole table, taking no predicate and honouring none", where, privileges, appRole, allowed)
		}
		held = privileges
	}
	// Equality, not containment: the loop above already refused anything wider,
	// so a difference here can only be a missing letter. PostgreSQL emits an
	// ACL's privileges in its own canonical order, and both allowlists are
	// written in that order, so the two strings compare directly.
	if required != "" && held != required {
		return fmt.Errorf("%s grants %q to %s — a tenant table must grant exactly %q, or the handler that reads or writes it answers `permission denied` at runtime while this gate passes", where, held, appRole, required)
	}

	var column, columnACL string
	err := conn.QueryRow(ctx, `
		SELECT a.attname, a.attacl::text FROM pg_attribute a
		 WHERE a.attrelid = $1 AND a.attacl IS NOT NULL LIMIT 1`, rel.oid).Scan(&column, &columnACL)
	if err == nil {
		return fmt.Errorf("%s.%s carries a column-level grant %s — column grants are outside the allowlist: they are invisible in the table's own ACL and split the privilege story in two", where, column, columnACL)
	}
	if err != pgx.ErrNoRows {
		return fmt.Errorf("reading %s's column grants: %w", where, err)
	}
	return nil
}
