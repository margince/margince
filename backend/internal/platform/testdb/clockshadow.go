// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

// The database applier: a now() of our own, ahead of pg_catalog's on the search
// path, so the integration lane can be reproduced at a moved clock on a machine
// whose wall clock must not move.
//
// It lives here because the lane's database admin goes through cmd/migrate's db
// verbs so that psql is not a host requirement, leaving no sanctioned shell path
// for DDL.
//
// Three things make the shadow reachable, and missing any one of them leaves it
// installed and inert: every statement answers at the real date under a lane
// reporting green.
//
//   - pg_catalog is named last. Unnamed, it is searched implicitly first and
//     nothing in a listed schema is ever reached.
//   - The schema is granted to PUBLIC. Postgres drops from a session's path any
//     schema that session's role lacks USAGE on, silently: the product's
//     queries run as the app role, and a schema owned by the migration role is
//     invisible to them.
//   - The shadow sits after public. current_schema() is the first schema on the
//     path that exists, and customfields and search both filter
//     information_schema by it, so leading with the shadow sends them looking
//     for the application's columns in a schema holding one function.
//
// ext stays off the path, as it is off every path the application connects
// with (extensionsqlscope_test.go and extmigrategate/role.go both rest on that).
// Putting it there would resolve a unit's unqualified ext_* name into ext
// only under this applier, and let the drift lane differ from the ordinary lane
// for a reason that has nothing to do with the clock.
//
// What it cannot move: CURRENT_TIMESTAMP, which resolves without consulting the
// search path, and the migrations' column DEFAULTs, which bound pg_catalog.now()
// before this existed. A green under this applier is weaker than the lane's.

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/shared/clockskew"
)

// shadowSchema holds the shadowed clock. Its own schema rather than public, so
// that dropping it removes the applier whole and nothing else.
const shadowSchema = "clockshadow"

// installClockShadow puts the shadowed now() in front of pg_catalog's for every
// connection opened to this database afterwards, and takes it back off for a
// run that is not using it.
//
// The setting is per-database rather than per-session because the tests reach
// the database through their own pools, not through this connection. A pool
// already dialled when this runs keeps the real clock, which is why it runs
// inside EnsureSchema's once, before schemaReady releases any pool.
func installClockShadow(ctx context.Context, owner *pgx.Conn) error {
	skew, err := clocktest.Skew()
	if err != nil {
		return err
	}
	var database string
	if err := owner.QueryRow(ctx, "SELECT current_database()").Scan(&database); err != nil {
		return fmt.Errorf("reading the database the shadow applies to: %w", err)
	}
	if skew.Applier != clockskew.Database {
		return clearClockShadow(ctx, owner, database)
	}

	// The day count is formatted in because a function BODY is a string literal
	// to the server and takes no parameter of its own. It is an int clockskew
	// has already bounded; nothing here comes from outside the process.
	shadow := fmt.Sprintf(`
		CREATE SCHEMA IF NOT EXISTS %[1]s;
		GRANT USAGE ON SCHEMA %[1]s TO PUBLIC;
		CREATE OR REPLACE FUNCTION %[1]s.now() RETURNS timestamptz
			LANGUAGE sql STABLE AS $shadow$
				SELECT pg_catalog.now() + pg_catalog.make_interval(days => %[2]d)
			$shadow$;`,
		shadowSchema, skew.Days)
	if _, err := owner.Exec(ctx, shadow); err != nil {
		return fmt.Errorf("installing the shadowed clock: %w", err)
	}

	// The catalog name is sanitised rather than formatted raw: it is an
	// identifier read back from the server, and the tree formats only
	// identifiers, only through Sanitize or as a compile-time literal.
	path := fmt.Sprintf(`ALTER DATABASE %s SET search_path = "$user", public, %s, pg_catalog`,
		pgx.Identifier{database}.Sanitize(), shadowSchema)
	if _, err := owner.Exec(ctx, path); err != nil {
		return fmt.Errorf("putting %s on %s's search path: %w", shadowSchema, database, err)
	}
	return nil
}

// clearClockShadow removes what a previous drift run left behind.
//
// ALTER DATABASE ... SET lives in pg_db_role_setting, which is in no schema:
// dropping and re-migrating the schema leaves it untouched, so one local
// `make backend-clock-drift` would otherwise leave every later ordinary run on
// the modified path and, once the shadow is reachable, 200 days into the
// future with nothing set and nothing printed. An ordinary run therefore clears
// the residue rather than inheriting it.
func clearClockShadow(ctx context.Context, owner *pgx.Conn, database string) error {
	var installed bool
	if err := owner.QueryRow(ctx,
		`SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1)`, shadowSchema).Scan(&installed); err != nil {
		return fmt.Errorf("looking for a previous run's shadowed clock: %w", err)
	}
	if !installed {
		return nil
	}
	reset := fmt.Sprintf(`ALTER DATABASE %s RESET search_path`, pgx.Identifier{database}.Sanitize())
	if _, err := owner.Exec(ctx, reset); err != nil {
		return fmt.Errorf("returning %s to the default search path: %w", database, err)
	}
	if _, err := owner.Exec(ctx, `DROP SCHEMA `+shadowSchema+` CASCADE`); err != nil {
		return fmt.Errorf("dropping a previous run's shadowed clock: %w", err)
	}
	return nil
}
