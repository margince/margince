// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package testdb

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

// CloneDBEnv is how a lane declares that this process's database is a
// throwaway clone it created from the migrated template for this package
// alone, and names it.
//
// The value is the DATABASE NAME, not a boolean, and the difference is the
// whole safety of the skip. Two callers inside the lane reach EnsureSchema
// with a database that is NOT the package's clone:
//
//   - make test-integration-serial runs every package against margince_test
//     ITSELF, the template each clone is copied from. Skipping the rebuild
//     there lets one package's residue persist into the template and get
//     file-copied into every later clone.
//   - tools/extmigrategate creates its own throwaway database mid-process and
//     migrates it through EnsureSchema, DEPENDING on the rebuild to strip the
//     lane clone's extension objects — its file comment states that as the
//     gate's premise.
//
// A boolean inherited from the lane's environment would have let the skip run
// in both. Comparing the name against current_database() refuses any database
// this process was not handed, and costs nothing.
//
// Unset means rebuild. That direction is not an accident either: a lane that
// forgets to declare its clone runs slower, while one that wrongly claims a
// shared database corrupts every package after it.
const CloneDBEnv = "MARGINCE_TEST_CLONE_DB"

// lookupCloneDB reads the lane's declaration. An empty value is treated as an
// absent one: a lane that expands an unset variable into the environment has
// declared nothing, and reading "" as a database name would compare it against
// current_database() and refuse in a second, less obvious place.
func lookupCloneDB() (string, bool) {
	declared, ok := os.LookupEnv(CloneDBEnv)
	if !ok || declared == "" {
		return "", false
	}
	return declared, true
}

// ledgerColumns is what the probe reads out of a tracking table, and the
// preflight counts these three by name.
//
// A template can predate content digests. Check the columns before reading them
// so EnsureSchema can refuse that clone with a rebuild command.
var ledgerColumns = []string{"version", "name", "content_digest"}

// preflightSQL answers, in one round trip, the two questions that decide
// whether the ledger is even readable: which database this connection is on,
// and whether both tracking tables carry the three columns the probe reads.
//
// information_schema rather than to_regclass + a second lookup: one query, and
// a table missing entirely counts zero columns exactly as a table missing the
// digest does — both are "cannot verify", and the probe owes them the same
// answer.
const preflightSQL = `
	SELECT current_database(),
	       (SELECT count(*) FROM information_schema.columns
	         WHERE table_schema = 'public'
	           AND table_name IN ('schema_migrations_core', 'schema_migrations_custom')
	           AND column_name = ANY($1))`

// reusableClone proves that the declared lane clone has this binary's migration
// ledger and no test rows. A reason refuses that clone before any schema drop.
// A database the lane did not declare has no reason and may be rebuilt.
//
// This checks the ledger, not hand-made DDL. The lane bounds that gap by copying
// a template built by its migrator into a private database for each package.
func reusableClone(ctx context.Context, owner *pgx.Conn) (bool, string, error) {
	declared, ok := lookupCloneDB()
	if !ok {
		return false, "", nil
	}
	var current string
	var columns int
	if err := owner.QueryRow(ctx, preflightSQL, ledgerColumns).Scan(&current, &columns); err != nil {
		return false, "", fmt.Errorf("probing the test database for a reusable schema: %w", err)
	}
	if current != declared {
		// Silent: see above. A suite that made its own database mid-process is
		// the intended shape here, not a mistake. What a SILENT refusal would
		// otherwise hide — a lane that exported the wrong name, turning the skip
		// off across every package — is caught by its own gate in
		// headprobe_integration_test.go instead.
		return false, "", nil
	}
	if want := 2 * len(ledgerColumns); columns != want {
		return false, ledgerRebuildReason(fmt.Sprintf("the migration ledger has %d of the %d columns the probe reads; this database did not record content digests", columns, want)), nil
	}
	if reason, err := ledgerAtHead(ctx, owner); err != nil {
		return false, "", err
	} else if reason != "" {
		return false, ledgerRebuildReason(reason), nil
	}
	// A role that cannot read a relation cannot prove the clone empty.
	reason, err := schemaEmpty(ctx, owner)
	if err != nil {
		return false, fmt.Sprintf("the clone could not be proved empty; check the template owner’s SELECT privileges and the schema: %v", err), nil
	}
	if reason != "" {
		return false, reason, nil
	}
	return true, "", nil
}

func ledgerRebuildReason(reason string) string {
	return reason + "; run `make test-db-up` in this worktree, then run the lane again to create a fresh clone"
}

// ledgerAtHead compares what each tracking table recorded against what this
// binary embeds, and returns the first disagreement in words.
func ledgerAtHead(ctx context.Context, owner *pgx.Conn) (string, error) {
	core, err := migrations.Core()
	if err != nil {
		return "", fmt.Errorf("loading the embedded core migrations: %w", err)
	}
	custom, err := migrations.Custom()
	if err != nil {
		return "", fmt.Errorf("loading the embedded custom migrations: %w", err)
	}
	extra, err := testExtensionNamespaces()
	if err != nil {
		return "", err
	}
	if reason, err := unlistedExtensionLedger(ctx, owner, extra); err != nil || reason != "" {
		return reason, err
	}
	for _, ns := range append([]dbmigrate.Namespace{core, custom}, extra...) {
		recorded, err := recordedMigrations(ctx, owner, "schema_migrations_"+ns.Name)
		if err != nil {
			return "", err
		}
		if reason := namespaceAtHead(ns, recorded); reason != "" {
			return reason, nil
		}
	}
	return "", nil
}

func recordedMigrations(ctx context.Context, owner *pgx.Conn, table string) (map[string]dbmigrate.LedgerRow, error) {
	// The table name is this package's own constant prefix plus a namespace
	// from the embedded migrations, never caller input, and dbmigrate.Up
	// interpolates the identical string.
	rows, err := owner.Query(ctx, `SELECT version, name, content_digest FROM `+pgx.Identifier{table}.Sanitize())
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", table, err)
	}
	defer rows.Close()

	recorded := map[string]dbmigrate.LedgerRow{}
	for rows.Next() {
		var version string
		var row dbmigrate.LedgerRow
		if err := rows.Scan(&version, &row.Name, &row.Digest); err != nil {
			return nil, fmt.Errorf("reading %s: %w", table, err)
		}
		recorded[version] = row
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("reading %s: %w", table, err)
	}
	return recorded, nil
}

// namespaceAtHead checks one namespace BOTH WAYS: every embedded migration is
// recorded under the same name with the same content, and nothing else is
// recorded at all.
//
// The second direction is not symmetry for its own sake. A clone taken from a
// template another branch built is at head for THAT branch — it carries schema
// objects this binary's migrations never created and its tests know nothing
// about, and every version this binary embeds is present, so a one-way check
// passes over it. Extra versions are the only evidence of that shape.
//
// It judges the ledger as the migrator leaves it. Migrate moves a declared
// renumber's old row, so that row is no reason to rebuild the template.
func namespaceAtHead(ns dbmigrate.Namespace, recorded map[string]dbmigrate.LedgerRow) string {
	recorded, err := dbmigrate.Reconciled(ns, recorded)
	if err != nil {
		return err.Error()
	}
	for _, m := range ns.Migrations {
		row, ok := recorded[m.Version]
		switch {
		case !ok:
			return fmt.Sprintf("%s %s_%s is embedded but not recorded — this database is behind this binary", ns.Name, m.Version, m.Name)
		case row.Name != m.Name:
			return fmt.Sprintf("%s %s was applied as %q but this binary carries %q there — the version was renumbered", ns.Name, m.Version, row.Name, m.Name)
		case row.Digest == nil:
			// The refusal an upgrading operator ACTUALLY meets, and the reason
			// the column-count branch above is not: trackingTable's
			// ALTER TABLE ... ADD COLUMN IF NOT EXISTS runs on every migrate, and
			// scripts/lib-testdb.sh ensure_template migrates the existing template
			// before any package starts — so by the time this probe runs the
			// column is there and only its rows are old. It carries the remedy for
			// that reason.
			return fmt.Sprintf("%s %s_%s recorded no content digest, so what it applied cannot be compared with what this binary embeds; this template predates content digests and rows are never back-filled", ns.Name, m.Version, m.Name)
		// The migrator and reuse check must accept the same admitted content.
		case !dbmigrate.ContentAdmitted(ns.Name, m, *row.Digest):
			return fmt.Sprintf("%s %s_%s was applied from different content than this binary embeds — the migration was edited after this database was migrated", ns.Name, m.Version, m.Name)
		}
	}
	if len(recorded) != len(ns.Migrations) {
		return fmt.Sprintf("%s records %d migrations where this binary embeds %d — this database was migrated from a different source tree", ns.Name, len(recorded), len(ns.Migrations))
	}
	return ""
}

// nonEmptyReportLimit is how many populated tables the refusal names before it
// summarises the rest. Enough to see the shape of the pollution, few enough
// that the line stays readable in a lane log.
const nonEmptyReportLimit = 5

// probedTable is one relation the emptiness probe reads, carrying the catalog
// fact that decides whether reading it answers about the TABLE or about the
// role doing the reading.
type probedTable struct {
	ident       string
	rowSecurity bool
}

// probedTables lists the relations a reset acts on, each with its row-security
// setting, in ONE catalog read.
//
// One read and not two, because the list the probe SCANS and the list it vets
// for readability have to be the same list by construction. A second query
// carrying its own predicate can match fewer relations, find no row rule among
// the ones it did match, and report a clean schema — under-recognition, which
// is the one direction this check must not fail in. The caller's refusal for an
// empty list closes the other direction.
func probedTables(ctx context.Context, owner *pgx.Conn) ([]probedTable, error) {
	rows, err := owner.Query(ctx,
		`SELECT quote_ident(n.nspname) || '.' || quote_ident(c.relname), c.relrowsecurity `+
			resetTables+` ORDER BY n.nspname, c.relname`)
	if err != nil {
		return nil, fmt.Errorf("listing data tables to prove the clone is empty: %w", err)
	}
	defer rows.Close()

	var tables []probedTable
	for rows.Next() {
		var table probedTable
		if err := rows.Scan(&table.ident, &table.rowSecurity); err != nil {
			return nil, fmt.Errorf("listing data tables to prove the clone is empty: %w", err)
		}
		tables = append(tables, table)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("listing data tables to prove the clone is empty: %w", err)
	}
	return tables, nil
}

// rowRuleRefusal names the relations whose rows a row-level rule can hide from
// this probe, and is a refusal rather than a note.
//
// EXISTS over a table with row-level security enabled answers about the ROLE
// running the query: a role that bypasses RLS is shown every row, one that does
// not is shown only what the policies admit, and under FORCE not even the owner
// is exempt. The same clone then reads empty to one lane and populated to the
// next, while EnsureSchema baselines every table size on whichever answer it
// got. A relation the probe cannot ask about the table itself is one it declines
// to vouch for.
func rowRuleRefusal(tables []probedTable) string {
	var filtered []string
	for _, table := range tables {
		if table.rowSecurity {
			filtered = append(filtered, table.ident)
		}
	}
	if len(filtered) == 0 {
		return ""
	}
	return fmt.Sprintf("%d table(s) carry row-level security (%s) — an emptiness check over them answers about the role reading rather than about the table. Remove those row rules from the test template or correct the migrations",
		len(filtered), summarise(filtered))
}

// schemaEmpty proves that every table a reset acts on currently holds no rows.
//
// EnsureSchema records emptySizes right after this, and reclaimBloat measures
// every later reset against that baseline — so a baseline taken over populated
// tables makes the reclaim compare growth against rows that were already
// there, for the life of the process. "At head" does not imply "empty": until
// PR #1994 the bench targets seeded a quarter of a million rows into the
// template every clone is copied from, and the unconditional rebuild this skip
// removes is what laundered that.
//
// It asks the same resetTables fragment the reset itself acts on, so a table
// the reset would empty can never be one this probe does not look at. And the
// verdict is only worth what the read behind it is worth, which is why the row
// rules come first: an EXISTS the catalog says can be filtered proves nothing
// about the table it ran against.
func schemaEmpty(ctx context.Context, owner *pgx.Conn) (string, error) {
	tables, err := probedTables(ctx, owner)
	if err != nil {
		return "", err
	}
	// A migrated database always has tables, so an empty list means the schema
	// is absent, which refuses reuse of that database.
	if len(tables) == 0 {
		return "no data tables in public or ext — this database carries a migration ledger but no schema; run `make test-db-up` in this worktree", nil
	}
	if reason := rowRuleRefusal(tables); reason != "" {
		return reason, nil
	}
	// One statement, and it selects each table's INDEX rather than its name:
	// the names are already quote_ident() output from pg_class and would need
	// literal-quoting to travel back as values, while an integer needs no
	// quoting at all and cannot be anything but what this loop wrote.
	branches := make([]string, 0, len(tables))
	for i, table := range tables {
		branches = append(branches, `SELECT `+strconv.Itoa(i)+` WHERE EXISTS (SELECT 1 FROM `+table.ident+`)`)
	}
	rows, err := owner.Query(ctx, strings.Join(branches, " UNION ALL "))
	if err != nil {
		return "", fmt.Errorf("probing the clone for rows: %w", err)
	}
	defer rows.Close()

	var populated []string
	for rows.Next() {
		var i int
		if err := rows.Scan(&i); err != nil {
			return "", fmt.Errorf("probing the clone for rows: %w", err)
		}
		populated = append(populated, tables[i].ident)
	}
	if err := rows.Err(); err != nil {
		return "", fmt.Errorf("probing the clone for rows: %w", err)
	}
	if len(populated) == 0 {
		return "", nil
	}
	return fmt.Sprintf("%d table(s) already hold rows (%s) — the template this clone came from is not empty. Remove stray test rows, or add migration-seeded reference tables to PreservedReferenceTables",
		len(populated), summarise(populated)), nil
}

// summarise names the first few entries and counts the rest.
func summarise(names []string) string {
	if len(names) <= nonEmptyReportLimit {
		return strings.Join(names, ", ")
	}
	return fmt.Sprintf("%s and %d more", strings.Join(names[:nonEmptyReportLimit], ", "), len(names)-nonEmptyReportLimit)
}
