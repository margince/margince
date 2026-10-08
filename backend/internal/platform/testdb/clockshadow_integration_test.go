// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
	"context"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/clockskew"
)

// The database applier is worth a test because every way it fails is silent: a
// shadow the path never reaches moves nothing, every statement answers at the
// real date, and the lane reports a green meaning the ordinary suite passed.
//
// The assertions run as the APP role, because that is the role the suite's
// queries use. Asked as the owner, the schema's own creator, all three of the
// reachability conditions in clockshadow.go answer yes whether or not they hold
// for anybody else.
//
// Each test gets its own database: the path is set per-database and would
// otherwise outlive the test for every suite that connected afterwards.

func TestTheDatabaseApplierMovesAnUnqualifiedNowForTheAppRole(t *testing.T) {
	ctx := context.Background()
	t.Setenv(clockskew.EnvVar, "database:200")

	owner, probe := shadowProbeDB(t)
	if err := installClockShadow(ctx, owner); err != nil {
		t.Fatalf("installing the shadow: %v", err)
	}

	// Dialled after the install, as the app role: ALTER DATABASE reaches the
	// sessions that connect next, and every test pool is held back until
	// EnsureSchema has run this.
	app := connectAs(t, appDSN(t), probe)
	var shifted, reserved, current, path string
	err := app.QueryRow(ctx, `SELECT (now() - pg_catalog.now())::text,
		(CURRENT_TIMESTAMP - pg_catalog.now())::text,
		current_schema()::text,
		array_to_string(current_schemas(true), ',')`).Scan(&shifted, &reserved, &current, &path)
	if err != nil {
		t.Fatalf("reading the shadowed clock: %v", err)
	}

	if shifted != "200 days" {
		t.Errorf("an unqualified now() is %s from the real clock for the app role, want 200 days. "+
			"Its effective path is %s — a schema missing from it was dropped for want of USAGE.", shifted, path)
	}
	// current_schema() is the first schema on the path that exists, and two
	// column probes filter information_schema by it.
	if current != "public" {
		t.Errorf("current_schema() = %q, want \"public\": every unqualified name resolves into the "+
			"shadow before reaching the application's own schema", current)
	}
	// ext is on no path the application connects with, and two gates rest on
	// that. This applier must not be the thing that changes it.
	//
	// Compared element by element rather than as a substring: the path holds
	// the expanded "$user", so a role or schema whose name merely contains the
	// three letters (extensions, context, an app role named for them) would
	// report a failure against a path that is correct.
	if slices.Contains(strings.Split(path, ","), "ext") {
		t.Errorf("the app role's effective path is %s, which volunteers ext: a unit's unqualified "+
			"ext_* name would resolve into it under this applier and nowhere else", path)
	}
	// The limit that makes a green here weaker than the machine applier's.
	if reserved != "00:00:00" {
		t.Errorf("CURRENT_TIMESTAMP moved by %s; it resolves without consulting the search path, so "+
			"a shift here means this applier reaches further than it is documented to", reserved)
	}
}

func TestEveryOtherApplierLeavesTheDatabaseClockAlone(t *testing.T) {
	ctx := context.Background()

	for _, value := range []string{"", "machine:200", "fixture:200"} {
		t.Run("applier="+value, func(t *testing.T) {
			t.Setenv(clockskew.EnvVar, value)

			owner, _ := shadowProbeDB(t)
			if err := installClockShadow(ctx, owner); err != nil {
				t.Fatalf("installing the shadow: %v", err)
			}
			if shadowInstalled(t, owner) {
				t.Errorf("the shadow schema exists under %q; only the database applier installs it", value)
			}
		})
	}
}

// A run that is not using the applier CLEARS what one left behind. The setting
// lives in pg_db_role_setting rather than in a schema, so re-migrating does not
// remove it and an ordinary run would otherwise inherit a previous drift run's
// path with nothing set and nothing printed.
func TestAnOrdinaryRunClearsAPreviousDriftRunsResidue(t *testing.T) {
	ctx := context.Background()
	t.Setenv(clockskew.EnvVar, "database:200")

	owner, probe := shadowProbeDB(t)
	if err := installClockShadow(ctx, owner); err != nil {
		t.Fatalf("installing the shadow: %v", err)
	}

	t.Setenv(clockskew.EnvVar, "")
	if err := installClockShadow(ctx, owner); err != nil {
		t.Fatalf("the ordinary run: %v", err)
	}

	if shadowInstalled(t, owner) {
		t.Error("the shadow schema survived an ordinary run")
	}
	app := connectAs(t, appDSN(t), probe)
	var shifted, path string
	if err := app.QueryRow(ctx, `SELECT (now() - pg_catalog.now())::text,
		array_to_string(current_schemas(true), ',')`).Scan(&shifted, &path); err != nil {
		t.Fatalf("reading the clock after the ordinary run: %v", err)
	}
	if shifted != "00:00:00" {
		t.Errorf("an ordinary run answers %s from the real clock on path %s — it inherited a drift "+
			"run's residue, which is a moved clock nobody asked for", shifted, path)
	}
}

func shadowInstalled(t *testing.T, owner *pgx.Conn) bool {
	t.Helper()

	var installed bool
	if err := owner.QueryRow(context.Background(),
		`SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = $1)`, shadowSchema).Scan(&installed); err != nil {
		t.Fatalf("looking for the shadow schema: %v", err)
	}
	return installed
}

// shadowProbeDB makes a throwaway database and hands back an OWNER connection
// to it plus its name, dropped when the test ends.
//
// Its own dial rather than the package's ownerConn, which runs EnsureSchema and
// would install the shadow on the shared clone.
func shadowProbeDB(t *testing.T) (*pgx.Conn, string) {
	t.Helper()

	// Named for the process, so two packages running side by side in the
	// parallel lane never share one probe.
	name := fmt.Sprintf("clockshadow_probe_%d", os.Getpid())
	ctx := context.Background()
	quoted := pgx.Identifier{name}.Sanitize()

	admin := connectAs(t, ownerDSN(t), "")
	if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+quoted+" WITH (FORCE)"); err != nil {
		t.Fatalf("clearing a previous probe database: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		t.Fatalf("creating the probe database: %v", err)
	}
	t.Cleanup(func() {
		// Through a connection to another database: a session cannot drop the
		// database it is connected to.
		cleanup := connectAs(t, ownerDSN(t), "")
		if _, err := cleanup.Exec(context.Background(), "DROP DATABASE IF EXISTS "+quoted+" WITH (FORCE)"); err != nil {
			t.Errorf("dropping the probe database: %v", err)
		}
	})
	return connectAs(t, ownerDSN(t), name), name
}

func ownerDSN(t *testing.T) string { return laneDSN(t, "MARGINCE_TEST_DSN") }

// appDSN is the role the product's queries run as, and the one the shadow has
// to be reachable from.
func appDSN(t *testing.T) string { return laneDSN(t, "MARGINCE_TEST_APP_DSN") }

func laneDSN(t *testing.T, name string) string {
	t.Helper()

	dsn := os.Getenv(name)
	if dsn == "" {
		t.Fatalf("%s not set — run `make db-up` (integration tests fail loudly, they never skip)", name)
	}
	return dsn
}

// connectAs dials the lane's cluster with dsn's credentials, swapping in
// database when it is named.
//
// The config is rebuilt rather than the DSN string, because reassembling a URL
// from its parts drops the password and the lane's DSNs carry one.
func connectAs(t *testing.T, dsn, database string) *pgx.Conn {
	t.Helper()

	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parsing the lane DSN: %v", err)
	}
	if database != "" {
		cfg.Database = database
	}
	conn, err := pgx.ConnectConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("connecting to %s as %s: %v", cfg.Database, cfg.User, err)
	}
	t.Cleanup(func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Errorf("closing the connection to %s: %v", cfg.Database, err)
		}
	})
	return conn
}
