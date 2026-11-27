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
	"maps"
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
	first := slices.IndexFunc(core.Migrations, func(m dbmigrate.Migration) bool {
		return m.Version > core.Baseline.Migration.Version
	})
	if first < 0 {
		return 0
	}
	return len(core.Migrations) - first
}

// An empty database takes the baseline. Each version it replaces is recorded
// under its own name and digest, so the datatask requirements and the repairs
// that read applied_at find their rows.
func TestAnEmptyDatabaseTakesTheBaselineAndRecordsWhatItReplaces(t *testing.T) {
	t.Parallel()
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
	t.Parallel()
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
// over a partial schema. Up stops before writing anything, and says what to
// do instead. One ledger row still sits under its version
// from before a renumber. A refusal that came after the renumber was written
// would show as a moved row.
func TestADatabasePartWayThroughTheHistoryIsRefused(t *testing.T) {
	t.Parallel()
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()
	core := coreWithBaseline(t)
	replaced := core.Baseline.Replaces
	built := replaced[len(replaced)-2].Version
	if _, err := dbmigrate.Up(ctx, conn, historyThrough(t, built)); err != nil {
		t.Fatalf("building a database one version short of the release: %v", err)
	}
	// The newest renumber this database already applied. A later one moved a
	// migration the history above has not reached, so its row is not there.
	var pair dbmigrate.Renumber
	for _, renumber := range core.Renumbered {
		if renumber.New <= built && renumber.New > pair.New {
			pair = renumber
		}
	}
	if pair.New == "" {
		t.Fatalf("no renumber falls within the history through %s; the renumbered row below would not exist", built)
	}
	moved, err := conn.Exec(ctx, `UPDATE schema_migrations_core SET version = $1, content_digest = $2 WHERE version = $3`,
		pair.Old, pair.OldDigest, pair.New)
	if err != nil || moved.RowsAffected() != 1 {
		t.Fatalf("recording %s under its old version %s: %v (%d rows)", pair.New, pair.Old, err, moved.RowsAffected())
	}
	before := coreLedger(t, conn)

	_, err = dbmigrate.Up(ctx, conn, core)
	refusal := "migrations the baseline " + core.Baseline.Migration.Version + "_" + core.Baseline.Migration.Name + " replaces"
	if err == nil || !strings.Contains(err.Error(), refusal) {
		t.Fatalf("err = %v, want the refusal of a partial history", err)
	}
	after := coreLedger(t, conn)
	if !maps.EqualFunc(before, after, func(a, b dbmigrate.LedgerRow) bool {
		return a.Name == b.Name && (a.Digest == nil) == (b.Digest == nil) && (a.Digest == nil || *a.Digest == *b.Digest)
	}) {
		t.Fatalf("the refused upgrade changed the ledger: %d rows before, %d after, %s now at %v",
			len(before), len(after), pair.Old, after[pair.Old])
	}
}

// Down never reverts the baseline: it reverts the replaced versions through
// their own files. Up then sees the baseline recorded and applies those files
// again, which is the same schema the baseline built.
func TestDownPastTheBaselineRevertsTheReplacedFilesAndUpReappliesThem(t *testing.T) {
	t.Parallel()
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

// CI's hold on the claim scripts/migration-baseline.sh verify proves byte for
// byte: both routes build the same catalog and the same reference rows.
func TestTheBaselineRouteBuildsWhatTheHistoryBuilds(t *testing.T) {
	t.Parallel()
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	custom, err := migrations.Custom()
	if err != nil {
		t.Fatalf("loading custom: %v", err)
	}
	history, err := migrations.CoreHistory()
	if err != nil {
		t.Fatalf("loading core history: %v", err)
	}
	built := map[string]string{}
	for route, core := range map[string]dbmigrate.Namespace{"history": history, "baseline": coreWithBaseline(t)} {
		resetSchema(t, conn)
		if _, err := dbmigrate.Up(context.Background(), conn, core, custom); err != nil {
			t.Fatalf("migrating by the %s route: %v", route, err)
		}
		built[route] = catalogProjection(t, conn) + referenceRowProjection(t, conn)
	}
	if added, removed := lineDiff(built["history"], built["baseline"]); len(added)+len(removed) > 0 {
		t.Fatalf("the baseline route differs from the history.\nonly the baseline route builds:\n  %s\nonly the history builds:\n  %s",
			strings.Join(added, "\n  "), strings.Join(removed, "\n  "))
	}
}

// referenceRowProjection lists the rows the migrations wrote, one line each and
// sorted, without its uuid and timestamptz columns: a run generates those.
func referenceRowProjection(t *testing.T, conn *pgx.Conn) string {
	t.Helper()
	ctx := context.Background()
	rows, err := conn.Query(ctx, `
		SELECT format('%I.%I', c.table_schema, c.table_name),
		       coalesce(array_agg(c.column_name::text) FILTER (WHERE c.data_type IN ('uuid', 'timestamp with time zone')), '{}')
		  FROM information_schema.columns c
		  JOIN information_schema.tables t USING (table_schema, table_name)
		 WHERE c.table_schema IN ('public', 'ext') AND t.table_type = 'BASE TABLE'
		   AND c.table_name NOT LIKE 'schema\_migrations\_%'
		 GROUP BY 1`)
	if err != nil {
		t.Fatalf("listing tables: %v", err)
	}
	generated := map[string][]string{}
	for rows.Next() {
		var table string
		var columns []string
		if err := rows.Scan(&table, &columns); err != nil {
			t.Fatalf("listing tables: %v", err)
		}
		generated[table] = columns
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("listing tables: %v", err)
	}
	var lines []string
	for table, columns := range generated {
		var values []string
		// The table is a catalog name quoted by format('%I'), never request input.
		if err := conn.QueryRow(ctx, `SELECT coalesce(array_agg((to_jsonb(r) - $1::text[])::text), '{}') FROM `+table+` r`,
			columns).Scan(&values); err != nil {
			t.Fatalf("reading %s: %v", table, err)
		}
		for _, v := range values {
			lines = append(lines, "row "+table+" "+v)
		}
	}
	slices.Sort(lines)
	return strings.Join(lines, "\n") + "\n"
}
