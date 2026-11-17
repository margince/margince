// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// The three ledgers core's baseline meets: an empty one, one that applied
// every version the baseline replaces, and one that applied only some. The
// verify script proves the schemas match. These prove what the migrator does
// to each ledger.

import (
	"context"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

func coreWithBaseline(t *testing.T) dbmigrate.Namespace {
	t.Helper()
	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	if core.Baseline == nil || len(core.Baseline.Replaces) == 0 {
		t.Fatal("core carries no baseline; every case below would pass vacuously")
	}
	return core
}

// historyThrough is core's history route stopped at version, which is the
// ledger a release that shipped version left behind.
func historyThrough(t *testing.T, version string) dbmigrate.Namespace {
	t.Helper()
	core, err := migrations.CoreHistory()
	if err != nil {
		t.Fatalf("loading core history: %v", err)
	}
	return namespaceThroughVersion(t, core, version)
}

func coreLedger(t *testing.T, conn *pgx.Conn) map[string]dbmigrate.LedgerRow {
	t.Helper()
	rows, err := conn.Query(context.Background(), `SELECT version, name, content_digest FROM schema_migrations_core`)
	if err != nil {
		t.Fatalf("reading the core ledger: %v", err)
	}
	defer rows.Close()
	ledger := map[string]dbmigrate.LedgerRow{}
	for rows.Next() {
		var version string
		var row dbmigrate.LedgerRow
		if err := rows.Scan(&version, &row.Name, &row.Digest); err != nil {
			t.Fatalf("reading the core ledger: %v", err)
		}
		ledger[version] = row
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("reading the core ledger: %v", err)
	}
	return ledger
}

// laterThanBaseline counts the core migrations a baseline install still
// applies one by one.
func laterThanBaseline(core dbmigrate.Namespace) int {
	return len(core.Migrations) - slices.IndexFunc(core.Migrations, func(m dbmigrate.Migration) bool {
		return m.Version > core.Baseline.Migration.Version
	})
}

// An empty database takes the baseline. It records each version the baseline
// replaces under that version's own name and digest, so datatask requirements
// and the repairs that read applied_at find their rows.
func TestAnEmptyDatabaseTakesTheBaselineAndRecordsWhatItReplaces(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	core := coreWithBaseline(t)

	applied, err := dbmigrate.Up(context.Background(), conn, core)
	if err != nil {
		t.Fatalf("up: %v", err)
	}
	if want := 1 + laterThanBaseline(core); applied != want {
		t.Fatalf("applied %d, want the baseline and the %d migrations after it", applied, want-1)
	}

	ledger := coreLedger(t, conn)
	if len(ledger) != len(core.Migrations)+1 {
		t.Fatalf("the ledger holds %d rows, want every migration (%d) and the baseline", len(ledger), len(core.Migrations))
	}
	for _, m := range append(slices.Clone(core.Migrations), core.Baseline.Migration) {
		row, ok := ledger[m.Version]
		if !ok || row.Name != m.Name || row.Digest == nil || *row.Digest != dbmigrate.Digest(m) {
			t.Fatalf("%s_%s is recorded as %+v, want its own name and digest", m.Version, m.Name, row)
		}
	}

	if again, err := dbmigrate.Up(context.Background(), conn, core); err != nil || again != 0 {
		t.Fatalf("a second up applied %d (err %v), want nothing", again, err)
	}
}

// A database the last release before the baseline built already has the
// schema. The baseline is recorded and never run: run, it would fail on the
// first table that already exists.
func TestADatabaseAtTheReleaseRecordsTheBaselineWithoutRunningIt(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	core := coreWithBaseline(t)
	last := core.Baseline.Replaces[len(core.Baseline.Replaces)-1].Version
	if _, err := dbmigrate.Up(context.Background(), conn, historyThrough(t, last)); err != nil {
		t.Fatalf("building the release's database: %v", err)
	}

	applied, err := dbmigrate.Up(context.Background(), conn, core)
	if err != nil {
		t.Fatalf("upgrading the release's database: %v", err)
	}
	if want := laterThanBaseline(core); applied != want {
		t.Fatalf("applied %d, want only the %d migrations after the baseline", applied, want)
	}
	row, ok := coreLedger(t, conn)[core.Baseline.Migration.Version]
	if !ok || row.Digest == nil || *row.Digest != dbmigrate.Digest(core.Baseline.Migration) {
		t.Fatalf("the baseline is recorded as %+v (present %v), want its name and digest", row, ok)
	}
}

// A database part-way through the replaced versions must not get the baseline
// over a partial schema. Up stops before writing anything, and names the
// release to upgrade with first.
func TestADatabasePartWayThroughTheHistoryIsRefused(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	core := coreWithBaseline(t)
	replaced := core.Baseline.Replaces
	if _, err := dbmigrate.Up(context.Background(), conn, historyThrough(t, replaced[len(replaced)-2].Version)); err != nil {
		t.Fatalf("building a database one release behind: %v", err)
	}
	before := coreLedger(t, conn)

	_, err := dbmigrate.Up(context.Background(), conn, core)
	if err == nil || !strings.Contains(err.Error(), "Upgrade it with release "+core.Baseline.Release+" first") {
		t.Fatalf("err = %v, want a refusal naming release %s", err, core.Baseline.Release)
	}
	if after := coreLedger(t, conn); len(after) != len(before) {
		t.Fatalf("the refused upgrade wrote %d ledger rows", len(after)-len(before))
	}
}

// Down never reverts the baseline: it reverts the replaced versions through
// their own files. Up then sees the baseline recorded and applies those files
// again, which is the same schema the baseline built.
func TestDownPastTheBaselineRevertsTheReplacedFilesAndUpReappliesThem(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	core := coreWithBaseline(t)
	if _, err := dbmigrate.Up(context.Background(), conn, core); err != nil {
		t.Fatalf("up: %v", err)
	}

	steps := laterThanBaseline(core) + 1
	if reverted, err := dbmigrate.Down(context.Background(), conn, core, steps); err != nil || reverted != steps {
		t.Fatalf("down reverted %d of %d (err %v)", reverted, steps, err)
	}
	ledger := coreLedger(t, conn)
	lastReplaced := core.Baseline.Replaces[len(core.Baseline.Replaces)-1].Version
	if _, ok := ledger[lastReplaced]; ok {
		t.Fatalf("down left %s recorded", lastReplaced)
	}
	if _, ok := ledger[core.Baseline.Migration.Version]; !ok {
		t.Fatal("down removed the baseline's row")
	}

	applied, err := dbmigrate.Up(context.Background(), conn, core)
	if err != nil || applied != steps {
		t.Fatalf("up applied %d (err %v), want the %d reverted migrations", applied, err, steps)
	}
}
