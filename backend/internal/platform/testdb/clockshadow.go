// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

// The database applier: a now() of our own, ahead of pg_catalog's on the search
// path, so the integration lane can be reproduced at a moved clock on a machine
// whose wall clock must not move.
//
// WHY IT IS HERE AND NOT IN A SHELL SCRIPT. The lane's database admin goes
// through cmd/migrate's db verbs precisely so that psql is not a host
// requirement (scripts/lib-testdb.sh), and there is no sanctioned shell path for
// arbitrary DDL. This is where the template's schema is already built, so this
// is where the shadow belongs.
//
// WHY pg_catalog MUST BE NAMED. Postgres searches pg_catalog implicitly BEFORE
// the listed schemas unless the path names it explicitly. A shadow sitting in
// public with the default path is therefore never reached — the path has to put
// clockshadow first and pg_catalog last, or the whole applier silently does
// nothing and the lane reports a green that means the ordinary suite passed.
//
// WHAT IT DOES NOT MOVE is clockskew.ShadowLimits, measured rather than assumed:
// CURRENT_TIMESTAMP resolves without consulting the search path, and the
// migrations' column DEFAULTs bound pg_catalog.now() when they ran, which was
// before this existed. That is why the lane prints that a green here is weaker
// than the machine applier's.

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
// connection opened to this database AFTERWARDS.
//
// The setting is per-database rather than per-session because the tests reach
// the database through their own pools, not through this connection. A pool
// already dialled when this runs keeps the real clock — which is why it runs
// inside EnsureSchema's once, before a package opens anything.
//
// A no-op under every other applier: the machine applier has already moved the
// clock this database reads, and shadowing on top of it would put the database
// 400 days out while the Go process stood at 200.
func installClockShadow(ctx context.Context, owner *pgx.Conn) error {
	skew, err := clocktest.Skew()
	if err != nil {
		return err
	}
	if skew.Applier != clockskew.Database {
		return nil
	}

	// The interval is built from the parsed day count, which is an int that
	// clockskew has already refused unless it is positive. It is formatted in
	// because a function BODY is a string literal to the server and takes no
	// parameter of its own; nothing here comes from outside the process.
	shadow := fmt.Sprintf(`
		CREATE SCHEMA IF NOT EXISTS %s;
		CREATE OR REPLACE FUNCTION %s.now() RETURNS timestamptz
			LANGUAGE sql STABLE AS $shadow$
				SELECT pg_catalog.now() + pg_catalog.make_interval(days => %d)
			$shadow$;`,
		shadowSchema, shadowSchema, skew.Days)
	if _, err := owner.Exec(ctx, shadow); err != nil {
		return fmt.Errorf("installing the shadowed clock: %w", err)
	}

	var database string
	if err := owner.QueryRow(ctx, "SELECT current_database()").Scan(&database); err != nil {
		return fmt.Errorf("reading the database to put the shadow on the path of: %w", err)
	}
	// The shadow goes AFTER public, not in front of it. current_schema() is the
	// first schema on the path that exists, so leading with clockshadow makes it
	// the current schema — and two column probes filter information_schema by
	// current_schema() (customfields/create.go, search/querystorage.go), so they
	// would look in the shadow schema and find none of the application's
	// columns. An unqualified CREATE TABLE would land there too. Measured both
	// ways: from public, current_schema() and an unqualified create both return
	// to public, and now() is still shifted, because what shadows pg_catalog is
	// naming it LAST rather than being first.
	//
	// The catalog name is sanitised rather than formatted raw: it is an
	// identifier read back from the server, and the tree formats only
	// identifiers, only through Sanitize or as a compile-time literal.
	path := fmt.Sprintf(`ALTER DATABASE %s SET search_path = "$user", public, %s, ext, pg_catalog`,
		pgx.Identifier{database}.Sanitize(), shadowSchema)
	if _, err := owner.Exec(ctx, path); err != nil {
		return fmt.Errorf("putting %s ahead of pg_catalog on %s's search path: %w", shadowSchema, database, err)
	}
	return nil
}
