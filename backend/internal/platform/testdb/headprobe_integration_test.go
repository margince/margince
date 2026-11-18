// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

// The head probe decides whether EnsureSchema may take an already-migrated
// clone at its word instead of rebuilding it. Every way that decision can be
// wrong is a package whose tests run against a schema nobody chose, so each of
// the probe's proofs is asserted here in the shape that would actually slip
// past it — a stale template, an edited migration, a renumbered one, a clone
// from another branch, a populated one — rather than in the shape that is easy
// to plant.

import (
	"context"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

// atHead is the ledger this binary would record for a namespace, which is what
// every arm below starts from and then breaks in exactly one way.
func atHead(t *testing.T, ns dbmigrate.Namespace) map[string]dbmigrate.LedgerRow {
	t.Helper()
	recorded := map[string]dbmigrate.LedgerRow{}
	for _, entry := range ns.LedgerAtHead() {
		recorded[entry.Version] = dbmigrate.LedgerRow{Name: entry.Name, Digest: new(entry.Digest)}
	}
	return recorded
}

func coreNamespace(t *testing.T) dbmigrate.Namespace {
	t.Helper()
	ns, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading the embedded core migrations: %v", err)
	}
	if len(ns.Migrations) == 0 {
		t.Fatal("the embedded core namespace is empty — every arm below would pass vacuously")
	}
	return ns
}

func TestAnAtHeadLedgerIsAccepted(t *testing.T) {
	t.Parallel()
	ns := coreNamespace(t)
	if reason := namespaceAtHead(ns, atHead(t, ns)); reason != "" {
		t.Fatalf("a ledger recording exactly this binary's migrations was refused: %s", reason)
	}
}

// A template that applied a declared renumber under its old version is one the
// migrator reconciles, so the probe must not rebuild it. The same row with
// content the source does not prove is the migrator's refusal, and the probe's.
func TestADeclaredRenumberIsJudgedAsTheMigratorLeavesIt(t *testing.T) {
	t.Parallel()
	ns := coreNamespace(t)
	if len(ns.Renumbered) == 0 {
		t.Fatal("the embedded core namespace declares no renumber — this test would pass vacuously")
	}
	pair := ns.Renumbered[0]
	ledger := atHead(t, ns)
	moved := ledger[pair.New]
	var asOld dbmigrate.Migration
	for _, m := range ns.Migrations {
		if m.Version == pair.New {
			asOld = m
		}
	}
	asOld.Version = pair.Old
	delete(ledger, pair.New)
	ledger[pair.Old] = dbmigrate.LedgerRow{Name: moved.Name, Digest: new(dbmigrate.Digest(asOld))}

	if reason := namespaceAtHead(ns, ledger); reason != "" {
		t.Fatalf("a ledger the migrator reconciles was refused: %s", reason)
	}

	ledger[pair.Old] = dbmigrate.LedgerRow{Name: moved.Name, Digest: moved.Digest}
	if reason := namespaceAtHead(ns, ledger); !strings.Contains(reason, "different content") {
		t.Fatalf("an old row whose digest does not prove the renumber was not refused for its content: %q", reason)
	}
}

// TestEveryWayALedgerCanLieIsRefused walks the shapes a tracking table can
// take that all look migrated. Each is a real failure mode. A template was
// built before the newest migration landed, or by the history, without the
// baseline's row. A migration was renumbered or edited under it, or a binary
// recorded no digest. A clone came from another branch's template, at head for
// that branch and holding objects this one never created.
func TestEveryWayALedgerCanLieIsRefused(t *testing.T) {
	t.Parallel()
	ns := coreNamespace(t)
	newest := ns.Migrations[len(ns.Migrations)-1]

	for _, tc := range []struct {
		name    string
		corrupt func(map[string]dbmigrate.LedgerRow)
		expects string
	}{
		{
			name:    "a migration this binary embeds was never applied",
			corrupt: func(l map[string]dbmigrate.LedgerRow) { delete(l, newest.Version) },
			expects: "behind this binary",
		},
		{
			name:    "the history built it, so the baseline is not recorded",
			corrupt: func(l map[string]dbmigrate.LedgerRow) { delete(l, ns.Baseline.Migration.Version) },
			expects: "behind this binary",
		},
		{
			name: "the version was applied under a different name",
			corrupt: func(l map[string]dbmigrate.LedgerRow) {
				row := l[newest.Version]
				row.Name += "_other"
				l[newest.Version] = row
			},
			expects: "renumbered",
		},
		{
			name: "the row records no content digest",
			corrupt: func(l map[string]dbmigrate.LedgerRow) {
				row := l[newest.Version]
				row.Digest = nil
				l[newest.Version] = row
			},
			expects: "no content digest",
		},
		{
			name: "the migration was edited after it was applied",
			corrupt: func(l map[string]dbmigrate.LedgerRow) {
				row := l[newest.Version]
				row.Digest = new(strings.Repeat("0", len(*row.Digest)))
				l[newest.Version] = row
			},
			expects: "different content",
		},
		{
			name: "the ledger records a migration this binary does not have",
			corrupt: func(l map[string]dbmigrate.LedgerRow) {
				l["9999999999"] = dbmigrate.LedgerRow{Name: "from_another_branch", Digest: new("unchecked")}
			},
			expects: "different source tree",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ledger := atHead(t, ns)
			tc.corrupt(ledger)
			reason := namespaceAtHead(ns, ledger)
			if reason == "" {
				t.Fatalf("the probe accepted a ledger that %s — EnsureSchema would have skipped the rebuild and every test in the package would run on that schema", tc.name)
			}
			if !strings.Contains(reason, tc.expects) {
				t.Errorf("the refusal does not say what is wrong: got %q, want it to mention %q", reason, tc.expects)
			}
		})
	}
}

// TestTheDigestSeparatesTheMigrationsParts pins the framing rather than the
// hash. Without length prefixes a version ending in digits and a name starting
// with them concatenate into the same bytes as the pair that splits them
// differently, and two different migrations would share one fingerprint.
func TestTheDigestSeparatesTheMigrationsParts(t *testing.T) {
	t.Parallel()
	a := dbmigrate.Migration{Version: "0001", Name: "ab", UpSQL: "u", DownSQL: "d"}
	b := dbmigrate.Migration{Version: "0001a", Name: "b", UpSQL: "u", DownSQL: "d"}
	if dbmigrate.Digest(a) == dbmigrate.Digest(b) {
		t.Fatal("two migrations whose parts concatenate identically share a digest — the framing is not separating them")
	}
	// The ledger column holds this verbatim, so its shape is part of the
	// contract: 64 lower-case hex characters, one sha256.
	if got := dbmigrate.Digest(a); len(got) != 64 || strings.Trim(got, "0123456789abcdef") != "" {
		t.Fatalf("the digest is %q, which is not the 64 hex characters of a sha256", got)
	}
	rolledBackDifferently := a
	rolledBackDifferently.DownSQL = "DROP TABLE something_else"
	if dbmigrate.Digest(a) == dbmigrate.Digest(rolledBackDifferently) {
		t.Fatal("two migrations with different down-migrations share a digest — the suites in backend/migrations run that half")
	}
}

// TestUpStampsTheDigestOfWhatItApplied closes the loop the pure arms above
// cannot: that what dbmigrate.Up WRITES into a tracking table is what Digest
// computes for the migration it just applied.
//
// It migrates a scratch namespace through the real migrator, in this process,
// rather than reading the core ledger — that ledger was stamped by whichever
// binary last built the template, so a probe of it would keep passing over a
// migrator that had stopped stamping until somebody rebuilt.
func TestUpStampsTheDigestOfWhatItApplied(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	owner := ownerConn(t)

	probe := dbmigrate.Migration{
		Version: "0001",
		Name:    "stamp_probe",
		UpSQL:   `SELECT 1`,
		DownSQL: `SELECT 1`,
	}
	ns := dbmigrate.Namespace{Name: "headprobe_stamp_probe", Migrations: []dbmigrate.Migration{probe}}
	table := "schema_migrations_" + ns.Name
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(), `DROP TABLE IF EXISTS `+table); err != nil {
			t.Errorf("dropping the scratch tracking table: %v", err)
		}
	})
	applied, err := dbmigrate.Up(ctx, owner, ns)
	if err != nil {
		t.Fatalf("applying the scratch namespace: %v", err)
	}
	if applied != 1 {
		t.Fatalf("the scratch namespace applied %d migrations, want 1 — nothing was stamped to read back", applied)
	}

	recorded, err := recordedMigrations(ctx, owner, table)
	if err != nil {
		t.Fatalf("reading %s: %v", table, err)
	}
	row, ok := recorded[probe.Version]
	if !ok {
		t.Fatalf("%s recorded no row for the migration that was just applied", table)
	}
	if row.Digest == nil {
		t.Fatal("dbmigrate.Up recorded no content digest — every clone would then be refused as unverifiable")
	}
	if want := dbmigrate.Digest(probe); *row.Digest != want {
		t.Fatalf("Up stamped %s where Digest computes %s — the probe compares two different things", *row.Digest, want)
	}
}

// TestThePreflightCountsTheLedgerColumnsByName proves preflightSQL asks what it
// claims to ask. A count that ignored its filter would return the same number
// for any column list, and the probe would then read an unverifiable ledger as
// a verified one.
func TestThePreflightCountsTheLedgerColumnsByName(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	owner := ownerConn(t)

	var current string
	var columns int
	if err := owner.QueryRow(ctx, preflightSQL, ledgerColumns).Scan(&current, &columns); err != nil {
		t.Fatalf("running the preflight: %v", err)
	}
	if current == "" {
		t.Fatal("the preflight did not report which database this connection is on")
	}
	if want := 2 * len(ledgerColumns); columns != want {
		t.Fatalf("the preflight found %d ledger columns on a migrated database, want %d — the probe would refuse every clone", columns, want)
	}

	absent := append(append([]string{}, ledgerColumns...), "a_column_no_tracking_table_has")
	if err := owner.QueryRow(ctx, preflightSQL, absent).Scan(&current, &columns); err != nil {
		t.Fatalf("running the preflight with an absent column: %v", err)
	}
	if want := 2 * len(ledgerColumns); columns != want {
		t.Fatalf("asking for a column no tracking table has changed the count to %d, want %d — the preflight is not counting by name", columns, want)
	}
}

// TestAPopulatedCloneIsNeverReused is the emptiness proof: a template that holds
// rows hands them to every test's copy.
func TestAPopulatedCloneIsNeverReused(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	owner := ownerConn(t)

	if _, err := owner.Exec(ctx, `INSERT INTO workspace (id) VALUES (gen_random_uuid())`); err != nil {
		t.Fatalf("seeding a row: %v", err)
	}
	reason, err := schemaEmpty(ctx, owner)
	if err != nil {
		t.Fatalf("probing for rows: %v", err)
	}
	if reason == "" {
		t.Fatal("the probe called a database holding a workspace row empty — every test's copy would start with those rows")
	}
	if !strings.Contains(reason, "public.workspace") {
		t.Errorf("the refusal does not name what it found: %q", reason)
	}
	if !strings.Contains(reason, "PreservedReferenceTables") || strings.Contains(reason, "make test-db-up") {
		t.Errorf("a populated clone needs a data or reference-table fix: %s", reason)
	}

	if reason, err := headAndEmpty(ctx, owner, 2*len(ledgerColumns)); err != nil || !strings.Contains(reason, "public.workspace") {
		t.Errorf("the whole check did not report what the emptiness step found (err=%v): %q", err, reason)
	}
}

// TestASchemalessDatabaseIsNotMistakenForAnEmptyOne is the other half of the
// emptiness proof, and the half a "no rows anywhere" check gets wrong for free.
//
// "Every table this reset touches holds no rows" is trivially true of a database
// with NO TABLES AT ALL. A probe that stopped at the row count would call such a
// database empty, agree to reuse it, and hand EnsureSchema a connection to
// nothing — where the first suite to run would fail on a missing relation rather
// than on the schema that was never built. So the absence of tables is a
// REFUSAL, phrased as its own reason, not a pass.
//
// A throwaway database, because the point is a database with no schema and the
// clone every other test in this package shares must keep its own.
func TestASchemalessDatabaseIsNotMistakenForAnEmptyOne(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	owner := ownerConn(t)

	const bare = "margince_headprobe_no_schema"
	if _, err := owner.Exec(ctx, `DROP DATABASE IF EXISTS `+bare); err != nil {
		t.Fatalf("clearing a previous run's throwaway database: %v", err)
	}
	if _, err := owner.Exec(ctx, `CREATE DATABASE `+bare); err != nil {
		t.Fatalf("creating the throwaway database: %v", err)
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(), `DROP DATABASE IF EXISTS `+bare); err != nil {
			t.Errorf("dropping the throwaway database: %v", err)
		}
	})

	cfg := owner.Config().Copy()
	cfg.Database = bare
	empty, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatalf("connecting to the throwaway database: %v", err)
	}
	defer func() {
		if err := empty.Close(context.Background()); err != nil {
			t.Errorf("closing the throwaway connection: %v", err)
		}
	}()

	reason, err := schemaEmpty(ctx, empty)
	if err != nil {
		t.Fatalf("probing a database with no schema: %v", err)
	}
	if reason == "" {
		t.Fatal("the probe called a database with no tables empty — reuse would hand every suite a connection to nothing, and the first missing relation would read as a product bug rather than as a schema that was never built")
	}
	if !strings.Contains(reason, "no data tables") {
		t.Errorf("the refusal does not say the schema is absent, which is what a reader has to act on: %q", reason)
	}
}

// TestACloneCarryingARowRuleIsNeverReused holds the readability half of the
// emptiness proof. EXISTS over a table with row-level security enabled answers
// about the ROLE running the probe rather than about the table — a bypassing
// role is shown every row, a non-bypassing one only what the policies admit,
// and under FORCE not even the owner is exempt. One clone then reads empty to
// one lane and populated to the next, while EnsureSchema baselines every table
// size on whichever answer it happened to get.
func TestACloneCarryingARowRuleIsNeverReused(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	owner := ownerConn(t)

	const planted = "ext.ext_row_rule_probe"
	if _, err := owner.Exec(ctx, `CREATE TABLE `+planted+` (id integer PRIMARY KEY)`); err != nil {
		t.Fatalf("planting a table a reset acts on: %v", err)
	}
	if _, err := owner.Exec(ctx, `ALTER TABLE `+planted+` ENABLE ROW LEVEL SECURITY, FORCE ROW LEVEL SECURITY`); err != nil {
		t.Fatalf("arming the planted table with a row rule: %v", err)
	}

	// The corpus first: a check that read a smaller list than the reset acts on
	// would find no row rule among what it did read and report a clean schema,
	// which is the one direction this refusal must not break in.
	tables, err := probedTables(ctx, owner)
	if err != nil {
		t.Fatalf("listing the tables the probe reads: %v", err)
	}
	if len(tables) == 0 {
		t.Fatal("the probe reads no tables at all on a migrated database, so every assertion below would pass vacuously")
	}
	var listed bool
	for _, table := range tables {
		if table.ident != planted {
			continue
		}
		listed = true
		if !table.rowSecurity {
			t.Fatalf("%s carries a row rule that the catalog read did not report", planted)
		}
	}
	if !listed {
		t.Fatalf("%s is a table a reset acts on and the probe does not list it", planted)
	}

	reason, err := schemaEmpty(ctx, owner)
	if err != nil {
		t.Fatalf("probing for rows: %v", err)
	}
	if reason == "" {
		t.Fatalf("the probe vouched for a clone carrying a row rule on %s — its emptiness answer changes with the role reading it", planted)
	}
	if !strings.Contains(reason, planted) {
		t.Errorf("the refusal does not name what it found: %q", reason)
	}
	if !strings.Contains(reason, "Remove those row rules") || strings.Contains(reason, "make test-db-up") {
		t.Errorf("a row-rule refusal needs a schema fix: %s", reason)
	}

	if _, err := owner.Exec(ctx, `DROP TABLE `+planted); err != nil {
		t.Fatalf("dropping the planted table: %v", err)
	}
	if reason, err := schemaEmpty(ctx, owner); err != nil || reason != "" {
		t.Fatalf("with the row rule gone the same clone was still refused (err=%v): %s", err, reason)
	}
}
