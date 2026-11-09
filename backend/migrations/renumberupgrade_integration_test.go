// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// A database migrated by an older head recorded some migrations under the
// versions they had then. These tests build such a database through the real
// migrator and upgrade it with today's namespaces.

import (
	"bufio"
	"context"
	"maps"
	"os"
	"slices"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
)

// oldHead returns the namespaces the old head shipped, built from today's
// files. A migration the manifest names at a declared old version gets today's
// content under that version, which is the content renumbered.txt claims it
// had. The old binary had no renumbers to reconcile, so none are attached.
func oldHead(t *testing.T) []dbmigrate.Namespace {
	t.Helper()
	core, custom := namespaces(t)
	text, err := os.ReadFile(oldHeadManifest)
	if err != nil {
		t.Fatalf("reading %s: %v", oldHeadManifest, err)
	}
	var old []dbmigrate.Namespace
	for _, ns := range []dbmigrate.Namespace{core, custom} {
		byVersion := map[string]dbmigrate.Migration{}
		for _, m := range ns.Migrations {
			byVersion[m.Version] = m
		}
		for _, p := range ns.Renumbered {
			m := byVersion[p.New]
			m.Version = p.Old
			byVersion[p.Old] = m
		}
		shipped := dbmigrate.Namespace{Name: ns.Name}
		lines := bufio.NewScanner(strings.NewReader(string(text)))
		for lines.Scan() {
			entry, ok := strings.CutPrefix(lines.Text(), ns.Name+"/")
			if !ok {
				continue
			}
			version, name, _ := strings.Cut(entry, "_")
			m, ok := byVersion[version]
			if !ok || m.Name != name {
				t.Fatalf("%s: the old head shipped %s, which is neither in the tree nor declared renumbered", oldHeadManifest, entry)
			}
			shipped.Migrations = append(shipped.Migrations, m)
		}
		slices.SortFunc(shipped.Migrations, func(a, b dbmigrate.Migration) int { return strings.Compare(a.Version, b.Version) })
		old = append(old, shipped)
	}
	return old
}

// below trims each namespace to the migrations sorting before version.
func below(nss []dbmigrate.Namespace, version string) []dbmigrate.Namespace {
	trimmed := make([]dbmigrate.Namespace, 0, len(nss))
	for _, ns := range nss {
		ns.Migrations = slices.DeleteFunc(slices.Clone(ns.Migrations), func(m dbmigrate.Migration) bool { return m.Version >= version })
		trimmed = append(trimmed, ns)
	}
	return trimmed
}

// migrateTo resets the schema and applies nss through the real migrator.
func migrateTo(t *testing.T, conn *pgx.Conn, nss []dbmigrate.Namespace) {
	t.Helper()
	resetSchema(t, conn)
	if _, err := dbmigrate.Up(context.Background(), conn, nss...); err != nil {
		t.Fatalf("building the old-head database: %v", err)
	}
}

// upgrade runs today's core and custom namespaces over whatever conn holds.
func upgrade(t *testing.T, conn *pgx.Conn) (dbmigrate.UpReport, error) {
	t.Helper()
	core, custom := namespaces(t)
	return dbmigrate.UpWithReport(context.Background(), conn, core, custom)
}

// ledger reads both tracking tables as namespace/version → digest.
func ledger(t *testing.T, conn *pgx.Conn) map[string]string {
	t.Helper()
	rows, err := conn.Query(context.Background(), `
		SELECT 'core/' || version, coalesce(content_digest, 'NULL') FROM schema_migrations_core
		UNION ALL
		SELECT 'custom/' || version, coalesce(content_digest, 'NULL') FROM schema_migrations_custom`)
	if err != nil {
		t.Fatalf("reading the ledger: %v", err)
	}
	recorded, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) ([2]string, error) {
		var pair [2]string
		err := row.Scan(&pair[0], &pair[1])
		return pair, err
	})
	if err != nil {
		t.Fatalf("reading the ledger: %v", err)
	}
	byVersion := make(map[string]string, len(recorded))
	for _, pair := range recorded {
		byVersion[pair[0]] = pair[1]
	}
	return byVersion
}

// headLedger is the ledger a fresh database at head records.
func headLedger(t *testing.T) map[string]string {
	t.Helper()
	core, custom := namespaces(t)
	want := map[string]string{}
	for _, ns := range []dbmigrate.Namespace{core, custom} {
		for _, m := range ns.Migrations {
			want[ns.Name+"/"+m.Version] = dbmigrate.Digest(m)
		}
	}
	return want
}

// freshCatalog is the catalog projection of a database built at head from empty.
func freshCatalog(t *testing.T, conn *pgx.Conn) string {
	t.Helper()
	resetSchema(t, conn)
	report, err := upgrade(t, conn)
	if err != nil {
		t.Fatalf("building head from empty: %v", err)
	}
	if report.Reconciled != 0 {
		t.Fatalf("a fresh database reconciled %d rows; it holds no old version to move", report.Reconciled)
	}
	if got := ledger(t, conn); !maps.Equal(got, headLedger(t)) {
		t.Fatal("a fresh database does not record exactly today's versions and digests")
	}
	return catalogProjection(t, conn)
}

// atHeadLike asserts what every successful upgrade owes: today's ledger and
// the catalog a fresh database has.
func atHeadLike(t *testing.T, conn *pgx.Conn, fresh string) {
	t.Helper()
	got, want := ledger(t, conn), headLedger(t)
	for version := range got {
		if _, ok := want[version]; !ok {
			t.Errorf("the upgraded ledger still records %s, which today's source does not ship", version)
		}
	}
	for version, digest := range want {
		if got[version] != digest {
			t.Errorf("the upgraded ledger records %s as %q, want %q", version, got[version], digest)
		}
	}
	if got := catalogProjection(t, conn); got != fresh {
		added, removed := lineDiff(fresh, got)
		t.Errorf("the upgraded database's catalog differs from a fresh one, so the old apply order left a schema difference:%s",
			formatDiff(added, removed, "the fresh database"))
	}
}

// declaredIn counts the declared old versions nss applies.
func declaredIn(t *testing.T, nss []dbmigrate.Namespace) int {
	t.Helper()
	core, _ := namespaces(t)
	olds := map[string]bool{}
	for _, p := range core.Renumbered {
		olds[p.Old] = true
	}
	n := 0
	for _, ns := range nss {
		for _, m := range ns.Migrations {
			if ns.Name == "core" && olds[m.Version] {
				n++
			}
		}
	}
	return n
}

func TestAnOldHeadDatabaseUpgradesToTheFreshSchema(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	fresh := freshCatalog(t, conn)

	old := oldHead(t)
	migrateTo(t, conn, old)
	report, err := upgrade(t, conn)
	if err != nil {
		t.Fatalf("upgrading the old head: %v", err)
	}
	if want := declaredIn(t, old); want == 0 || report.Reconciled != want {
		t.Errorf("reconciled %d rows, want the %d declared old versions the old head applied", report.Reconciled, want)
	}
	atHeadLike(t, conn, fresh)

	again, err := upgrade(t, conn)
	if err != nil || again != (dbmigrate.UpReport{}) {
		t.Fatalf("a second upgrade = %+v, %v; want nothing to do", again, err)
	}

	core, _ := namespaces(t)
	reverted, err := dbmigrate.Down(context.Background(), conn, core, 1)
	if err != nil || reverted != 1 {
		t.Fatalf("Down 1 after the reconcile = %d, %v", reverted, err)
	}
	if report, err := upgrade(t, conn); err != nil || report.Applied != 1 {
		t.Fatalf("re-applying the reverted migration = %+v, %v", report, err)
	}
}

// The old database stopped halfway through the renumbered block: some old
// versions are applied, the rest of the block is still to run.
func TestAPartlyMigratedOldHeadUpgradesToTheFreshSchema(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	fresh := freshCatalog(t, conn)

	partial := below(oldHead(t), "1793847156")
	migrateTo(t, conn, partial)
	report, err := upgrade(t, conn)
	if err != nil {
		t.Fatalf("upgrading the partial old head: %v", err)
	}
	if want := declaredIn(t, partial); want == 0 || report.Reconciled != want {
		t.Errorf("reconciled %d rows, want the %d declared old versions the partial head applied", report.Reconciled, want)
	}
	atHeadLike(t, conn, fresh)
}

// Half the old rows already sit at their new version, as a reconcile by hand
// or an earlier binary could leave them. The rest still move, and a second
// run changes nothing.
func TestAHalfReconciledLedgerFinishesAndStaysFinished(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	fresh := freshCatalog(t, conn)

	migrateTo(t, conn, oldHead(t))
	core, _ := namespaces(t)
	byVersion := map[string]dbmigrate.Migration{}
	for _, m := range core.Migrations {
		byVersion[m.Version] = m
	}
	moved := 0
	for i, p := range core.Renumbered {
		if i%2 == 1 {
			continue
		}
		tag, err := conn.Exec(context.Background(),
			`UPDATE schema_migrations_core SET version = $2, content_digest = $3 WHERE version = $1`,
			p.Old, p.New, dbmigrate.Digest(byVersion[p.New]))
		if err != nil {
			t.Fatalf("moving %s by hand: %v", p.Old, err)
		}
		moved += int(tag.RowsAffected())
	}
	if moved == 0 {
		t.Fatal("no old row was moved by hand, so this is the plain upgrade again")
	}

	if _, err := upgrade(t, conn); err != nil {
		t.Fatalf("finishing a half-reconciled ledger: %v", err)
	}
	atHeadLike(t, conn, fresh)
	again, err := upgrade(t, conn)
	if err != nil || again != (dbmigrate.UpReport{}) {
		t.Fatalf("a second upgrade = %+v, %v; want nothing to do", again, err)
	}
}

// Each refusal is an old row the renumber cannot be proved from. Up must
// stop before it moves any row or applies any migration.
func TestAnUnprovableOldRowStopsTheUpgradeBeforeAnyChange(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	old := oldHead(t)
	migrateTo(t, conn, old)
	core, _ := namespaces(t)
	pair := core.Renumbered[slices.IndexFunc(core.Renumbered, func(p dbmigrate.Renumber) bool { return p.Old == "1793847151" })]
	ctx := context.Background()

	for _, tc := range []struct {
		name, corrupt, restore, expects string
	}{
		{
			name:    "an edited old row",
			corrupt: `UPDATE schema_migrations_core SET content_digest = repeat('0', 64) WHERE version = '` + pair.Old + `'`,
			expects: "different content",
		},
		{
			name:    "an old row without a digest",
			corrupt: `UPDATE schema_migrations_core SET content_digest = NULL WHERE version = '` + pair.Old + `'`,
			expects: "no content digest",
		},
		{
			name: "both rows, the new one recording other content",
			corrupt: `INSERT INTO schema_migrations_core (version, name, content_digest)
				VALUES ('` + pair.New + `', '` + pair.Name + `', repeat('0', 64))`,
			restore: `DELETE FROM schema_migrations_core WHERE version = '` + pair.New + `'`,
			expects: "also holds the new version",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			before := ledger(t, conn)
			catalog := catalogDigest(t, conn)
			if _, err := conn.Exec(ctx, tc.corrupt); err != nil {
				t.Fatalf("corrupting the ledger: %v", err)
			}
			corrupted := ledger(t, conn)
			t.Cleanup(func() {
				restore := tc.restore
				if restore == "" {
					restore = `UPDATE schema_migrations_core SET content_digest = '` + before["core/"+pair.Old] + `' WHERE version = '` + pair.Old + `'`
				}
				if _, err := conn.Exec(ctx, restore); err != nil {
					t.Errorf("restoring the ledger: %v", err)
				}
			})

			report, err := upgrade(t, conn)
			if err == nil || !strings.Contains(err.Error(), tc.expects) || !strings.Contains(err.Error(), pair.Old+"_"+pair.Name) {
				t.Fatalf("upgrade = %+v, %v; want a refusal naming %s_%s and %q", report, err, pair.Old, pair.Name, tc.expects)
			}
			if report != (dbmigrate.UpReport{}) {
				t.Errorf("the refused upgrade reported %+v, want nothing done", report)
			}
			if !maps.Equal(ledger(t, conn), corrupted) {
				t.Error("the refused upgrade changed the ledger")
			}
			if catalogDigest(t, conn) != catalog {
				t.Error("the refused upgrade changed the schema")
			}
		})
	}
}

// A fresh database built up to the repair must already hold the bodies the
// repair writes. So the repair changes nothing there, and a later change to the
// meeting rule cannot leave it writing a stale body unnoticed.
func TestTheActivityClockRepairWritesTheBodiesAFreshDatabaseHas(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	repair, _ := repairAndPrior(t)
	_, bodies := functionStatements(repair.UpSQL)
	core, custom := namespaces(t)
	migrateTo(t, conn, below([]dbmigrate.Namespace{core, custom}, activityClockRepair))

	for name, body := range bodies {
		var live string
		if err := conn.QueryRow(context.Background(),
			`SELECT prosrc FROM pg_proc WHERE proname = $1 AND pronamespace = 'public'::regnamespace`, name).Scan(&live); err != nil {
			t.Fatalf("reading %s: %v", name, err)
		}
		if live != body {
			t.Errorf("%s: the repair writes a body the fresh database built before it does not have", name)
		}
	}
}
