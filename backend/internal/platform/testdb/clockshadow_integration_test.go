// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
	"context"
	"fmt"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/clockskew"
)

// The database applier is worth a test because its failure mode is silence: a
// shadow the search path never reaches moves nothing, every statement answers
// at the real date, and the lane reports a green that says only that the
// ordinary suite passed. Nothing about that reads as broken.
//
// It runs against its own database rather than the package's clone, because the
// path change is per-database and would otherwise outlive the test for every
// suite that connects afterwards.

func TestTheDatabaseApplierMovesAnUnqualifiedNow(t *testing.T) {
	ctx := context.Background()
	t.Setenv(clockskew.EnvVar, "database:200")

	conn, probe := shadowProbeDB(t)
	if err := installClockShadow(ctx, conn); err != nil {
		t.Fatalf("installing the shadow: %v", err)
	}

	// A CONNECTION OPENED AFTERWARDS, which is the whole point: ALTER DATABASE
	// reaches the sessions that dial next, and the pools every test uses are
	// held back until EnsureSchema has run this.
	fresh := connectTo(t, probe)
	var shifted, reserved, current string
	err := fresh.QueryRow(ctx, `SELECT (now() - pg_catalog.now())::text,
		(CURRENT_TIMESTAMP - pg_catalog.now())::text, current_schema()::text`).
		Scan(&shifted, &reserved, &current)
	if err != nil {
		t.Fatalf("reading the shadowed clock: %v", err)
	}

	// The shadow must not become the CURRENT schema. current_schema() is the
	// first schema on the path that exists, and two column probes filter
	// information_schema by it, so a shadow in front of public sends them
	// looking for the application's columns in a schema that holds one function
	// — and an unqualified CREATE TABLE would land there as well.
	if current != "public" {
		t.Errorf("current_schema() = %q, want \"public\" — the shadow is ahead of public on the "+
			"path, so every unqualified name resolves into it before reaching the application's", current)
	}

	if shifted != "200 days" {
		t.Errorf("an unqualified now() is %s from the real clock, want 200 days — the shadow is not "+
			"on the search path ahead of pg_catalog, so the applier moves nothing", shifted)
	}
	// Stated as an assertion rather than left to prose, because this is the
	// limit that makes a green here weaker than the machine applier's, and
	// clockskew.ShadowLimits promises it in words.
	if reserved != "00:00:00" {
		t.Errorf("CURRENT_TIMESTAMP moved by %s; it resolves without consulting the search path, so "+
			"a shift here means this test is no longer measuring what ShadowLimits claims", reserved)
	}
}

func TestEveryOtherApplierLeavesTheDatabaseClockAlone(t *testing.T) {
	ctx := context.Background()

	for _, value := range []string{"", "machine:200", "fixture:200"} {
		t.Run("applier="+value, func(t *testing.T) {
			t.Setenv(clockskew.EnvVar, value)

			conn, _ := shadowProbeDB(t)
			if err := installClockShadow(ctx, conn); err != nil {
				t.Fatalf("installing the shadow: %v", err)
			}
			var schemas int
			if err := conn.QueryRow(ctx,
				`SELECT count(*) FROM pg_namespace WHERE nspname = $1`, shadowSchema).Scan(&schemas); err != nil {
				t.Fatalf("looking for the shadow schema: %v", err)
			}
			if schemas != 0 {
				t.Errorf("the shadow was installed under %q. The machine applier already moved the clock "+
					"this database reads, so shadowing on top of it stands the database 400 days out "+
					"while the Go process stands at 200.", value)
			}
		})
	}
}

// shadowProbeDB makes a throwaway database and hands back an owner connection
// to it plus its name, dropped when the test ends.
func shadowProbeDB(t *testing.T) (*pgx.Conn, string) {
	t.Helper()

	// Named for the process, so two packages running side by side in the
	// parallel lane never share one probe.
	name := fmt.Sprintf("clockshadow_probe_%d", os.Getpid())
	ctx := context.Background()
	quoted := pgx.Identifier{name}.Sanitize()

	admin := connectTo(t, "")
	if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+quoted+" WITH (FORCE)"); err != nil {
		t.Fatalf("clearing a previous probe database: %v", err)
	}
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		t.Fatalf("creating the probe database: %v", err)
	}
	t.Cleanup(func() {
		// Through a connection of its own, to another database: a session
		// cannot drop the database it is connected to.
		cleanup := connectTo(t, "")
		if _, err := cleanup.Exec(context.Background(), "DROP DATABASE IF EXISTS "+quoted+" WITH (FORCE)"); err != nil {
			t.Errorf("dropping the probe database: %v", err)
		}
	})
	return connectTo(t, name), name
}

// connectTo dials the lane's cluster, swapping in database when it is named.
//
// The config is rebuilt rather than the DSN string, because reassembling a URL
// from its parts drops the password — and the lane's owner DSN carries one.
func connectTo(t *testing.T, database string) *pgx.Conn {
	t.Helper()

	dsn := os.Getenv("MARGINCE_TEST_DSN")
	if dsn == "" {
		t.Fatal("MARGINCE_TEST_DSN not set — run `make db-up` (integration tests fail loudly, they never skip)")
	}
	cfg, err := pgx.ParseConfig(dsn)
	if err != nil {
		t.Fatalf("parsing the owner DSN: %v", err)
	}
	if database != "" {
		cfg.Database = database
	}
	conn, err := pgx.ConnectConfig(context.Background(), cfg)
	if err != nil {
		t.Fatalf("connecting to %s: %v", cfg.Database, err)
	}
	t.Cleanup(func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Errorf("closing the connection to %s: %v", cfg.Database, err)
		}
	})
	return conn
}
