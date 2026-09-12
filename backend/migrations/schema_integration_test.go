// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package migrations_test

// Integration lane (make test-integration): exercises the real schema on
// Postgres 16 — apply/reverse/re-apply, the version bump (data-model §1.3a),
// audit_log append-only (§11), and the signal-visibility backfill. §1.3's
// ∅-query and GUC-unset-deny gates were the RLS mechanism's own proof and
// went with it when core retired RLS (ADR-0091 §8); what stands in their place is
// rbacgate_test.go over platform/auth. Fails loudly when the database is
// missing rather than skipping (a skipped security gate looks exactly like
// a passing one).

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/margince/margince/backend/internal/platform/dbmigrate"
	"github.com/margince/margince/backend/migrations"
)

// ownerDSN administers the throwaway test database; appDSNFmt is the
// non-owner runtime role RLS must bind.
func dsns(t *testing.T) (owner string, appFmt string) {
	t.Helper()
	owner = os.Getenv("MARGINCE_TEST_DSN")
	if owner == "" {
		t.Fatal("MARGINCE_TEST_DSN is not set — run `make db-up` and try again (integration tests fail loudly, they never skip)")
	}
	return owner, os.Getenv("MARGINCE_TEST_APP_DSN")
}

func connect(t *testing.T, dsn string) *pgx.Conn {
	t.Helper()
	conn, err := pgx.Connect(context.Background(), dsn)
	if err != nil {
		t.Fatalf("connecting to %s: %v", dsn, err)
	}
	t.Cleanup(func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Errorf("closing %s connection: %v", dsn, err)
		}
	})
	return conn
}

func migrateAll(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core migrations: %v", err)
	}
	custom, err := migrations.Custom()
	if err != nil {
		t.Fatalf("loading custom migrations: %v", err)
	}
	if _, err := dbmigrate.Up(context.Background(), conn, core, custom); err != nil {
		t.Fatalf("migrate up: %v", err)
	}
}

// resetSchema drops everything so each test run starts clean.
//
// BOTH schemas the migrations own, not just public. Every clone is copied from
// the migrated template (scripts/lib-testdb.sh), so ext arrives already created
// and owned by margince_owner; leaving it would hand 0202 a schema it did not
// create, and its `GRANT USAGE ON SCHEMA ext` would then fail for a migration
// role that is not the owner. Dropping it is the same move public already gets,
// for the same reason: the role under test must build what it owns.
//
// CASCADE here, where the migration's own down half deliberately uses RESTRICT:
// this is a test reset primitive over a throwaway clone, and refusing to clear
// a leftover extension table would only strand the next run. RESTRICT belongs
// in 0213_ext_schema.down.sql, where the data is real.
func resetSchema(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	ctx := context.Background()
	if _, err := conn.Exec(ctx, `DROP SCHEMA IF EXISTS ext CASCADE`); err != nil {
		t.Fatalf("resetting the ext schema: %v", err)
	}
	if _, err := conn.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public;`); err != nil {
		t.Fatalf("resetting schema: %v", err)
	}
	if _, err := conn.Exec(ctx, `GRANT USAGE ON SCHEMA public TO margince_app`); err != nil {
		t.Fatalf("re-granting schema usage: %v", err)
	}
}

func TestMigrations_applyReverseReapply(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}

	applied, err := dbmigrate.Up(ctx, conn, core)
	if err != nil {
		t.Fatalf("first up: %v", err)
	}
	if applied != len(core.Migrations) {
		t.Fatalf("applied %d, want %d", applied, len(core.Migrations))
	}

	// Idempotent: a second run applies nothing.
	applied, err = dbmigrate.Up(ctx, conn, core)
	if err != nil {
		t.Fatalf("second up: %v", err)
	}
	if applied != 0 {
		t.Fatalf("re-run applied %d, want 0", applied)
	}

	// Rows FIRST, and specifically in the append-only ledgers. An empty schema
	// reverses through anything: a down half that backfills with an UPDATE
	// aborts on the ledgers' immutability trigger, and a FOR EACH ROW trigger
	// never fires on zero rows — so this suite would report a rollback that
	// cannot happen on any installation that has ever done anything.
	seedLedgerRowsForReversal(t, conn)

	// Every migration reverses (B-EP02.1b), then the schema re-applies.
	reverted, err := dbmigrate.Down(ctx, conn, core, len(core.Migrations))
	if err != nil {
		t.Fatalf("down: %v", err)
	}
	if reverted != len(core.Migrations) {
		t.Fatalf("reverted %d, want %d", reverted, len(core.Migrations))
	}
	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatalf("re-apply after full down: %v", err)
	}
}

// seedWorkspace inserts a workspace.
//
// It writes no column at all: the row is identity and lifecycle now, every
// value on it defaulted. It used to probe the catalog for `workspace.name`,
// then wrote `slug` once that column was the last writable one — ADR-0091
// retired that too, and the whole INSERT went with it.
//
// label names the caller in the failure message, which is the only thing left
// distinguishing these seeds from one another.
func seedWorkspace(t *testing.T, conn *pgx.Conn, label string) string {
	t.Helper()
	var id string
	if err := conn.QueryRow(context.Background(),
		`INSERT INTO workspace DEFAULT VALUES RETURNING id`).Scan(&id); err != nil {
		t.Fatalf("seeding workspace %s: %v", label, err)
	}
	return id
}

// inTx runs fn inside ONE transaction, which is the shape the production
// database.WithWorkspaceTx seam gives a caller: every statement a module issues
// addresses the transaction rather than the bare pool. It binds no session
// setting, because there is none left to bind.
func inTx(t *testing.T, conn *pgx.Conn, fn func(pgx.Tx) error) error {
	t.Helper()
	ctx := context.Background()
	tx, err := conn.Begin(ctx)
	if err != nil {
		t.Fatalf("begin: %v", err)
	}
	//craft:ignore swallowed-errors error-path safety net: after the Commit below this rollback is a designed no-op, and fn's error already reached the caller
	defer func() { _ = tx.Rollback(ctx) }()
	if err := fn(tx); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func TestVersionBumpAndSkewSemantics(t *testing.T) {
	ownerDSN, appDSN := dsns(t)
	owner := connect(t, ownerDSN)
	headSchema(t, owner)
	// The installation's workspace. No row below names it any more, but a
	// schema holding none at all is not a state any installation is in.
	seedWorkspace(t, owner, "tenant-v")

	app := connect(t, appDSN)
	ctx := context.Background()

	var id string
	var version int64
	if err := inTx(t, app, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`INSERT INTO contact (full_name, source, captured_by) VALUES ('Vera', 'test', 'human:test') RETURNING id, version`,
		).Scan(&id, &version)
	}); err != nil {
		t.Fatalf("inserting contact: %v", err)
	}
	if version != 1 {
		t.Fatalf("fresh row version = %d, want 1", version)
	}

	// The trigger bumps version on every UPDATE (data-model §1.3a).
	if err := inTx(t, app, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			`UPDATE contact SET title = 'CTO' WHERE id = $1 RETURNING version`, id).Scan(&version)
	}); err != nil {
		t.Fatalf("updating contact: %v", err)
	}
	if version != 2 {
		t.Fatalf("version after update = %d, want 2", version)
	}

	// The If-Match write shape: a stale version matches zero rows.
	if err := inTx(t, app, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx,
			`UPDATE contact SET title = 'CEO' WHERE id = $1 AND version = $2`, id, int64(1))
		if err != nil {
			t.Fatalf("stale update: %v", err)
		}
		if tag.RowsAffected() != 0 {
			t.Error("stale If-Match version updated a row; must affect 0 → 409 version_skew")
		}
		return nil
	}); err != nil {
		t.Fatalf("stale-version tx: %v", err)
	}
}

func TestAuditLogIsAppendOnly(t *testing.T) {
	ownerDSN, appDSN := dsns(t)
	owner := connect(t, ownerDSN)
	headSchema(t, owner)
	// The installation's workspace, present for the reason the sibling test
	// above states.
	seedWorkspace(t, owner, "tenant-audit")

	app := connect(t, appDSN)
	ctx := context.Background()

	var id string
	if err := inTx(t, app, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx,
			// entity_id is NOT NULL since 0075 (audit_log is record-mutations-only).
			`INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id)
			 VALUES ('human', 'human:test', 'create', 'contact', uuidv7()) RETURNING id`).Scan(&id)
	}); err != nil {
		t.Fatalf("seeding an audit row: %v", err)
	}

	for _, stmt := range []string{
		`UPDATE audit_log SET actor_id = 'tampered' WHERE id = $1`,
		`DELETE FROM audit_log WHERE id = $1`,
	} {
		err := inTx(t, app, func(tx pgx.Tx) error {
			_, err := tx.Exec(ctx, stmt, id)
			return err
		})
		var pgErr *pgconn.PgError
		if err == nil {
			t.Errorf("%q succeeded; audit_log must be append-only", stmt)
		} else if !errors.As(err, &pgErr) {
			t.Errorf("%q failed with %v, want a loud database error", stmt, err)
		}
	}
}

// seedLedgerRowsForReversal plants one row in each append-only ledger, at head,
// so the reversal above has something a row-level trigger can refuse.
//
// The two ledgers are the only tables in the schema that forbid UPDATE and
// DELETE outright, which makes them the only ones whose down half cannot
// backfill the ordinary way. That asymmetry is invisible against an empty
// table, and an empty table is what this suite used to reverse.
func seedLedgerRowsForReversal(t *testing.T, conn *pgx.Conn) {
	t.Helper()
	ctx := context.Background()
	// A workspace first, because that is the only state these rows exist in:
	// both ledgers are written inside a workspace-bound transaction, and the
	// foreign key the rollback restores would have refused a row without one.
	seedWorkspace(t, conn, "ledger-reversal")
	if _, err := conn.Exec(ctx, `
		INSERT INTO audit_log (actor_type, actor_id, action, entity_type, entity_id)
		VALUES ('system', 'system:reversal-fixture', 'create', 'workspace', gen_random_uuid())`); err != nil {
		t.Fatalf("seeding the audit_log row this reversal must survive: %v", err)
	}
	if _, err := conn.Exec(ctx, `
		INSERT INTO system_log (actor_type, actor_id, action)
		VALUES ('system', 'system:reversal-fixture', 'reversal_fixture')`); err != nil {
		t.Fatalf("seeding the system_log row this reversal must survive: %v", err)
	}
}

// Down REFUSES a version whose applied content is not the content this binary
// holds — driven through dbmigrate.Down rather than against the predicate, so
// it proves the production revert path actually consults the digest.
//
// Up records a content_digest and, before this, nothing read it: the column
// held the evidence and no code acted on it (#2141). The down half is the
// sharper one. Running the CURRENT rollback against a schema the OLD
// up-migration built is a schema CHANGE made on a false premise — it drops
// what the source names rather than what the database has, then deletes the row
// that was the only record of what it did apply.
func TestMigrations_downRefusesAnEditedMigration(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}
	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Fatalf("up: %v", err)
	}

	// The edit a contributor would make: the newest migration's rollback,
	// changed after this database applied it. Its UP is untouched, so the
	// database looks current by every other measure the ledger keeps.
	edited := core
	edited.Migrations = append([]dbmigrate.Migration(nil), core.Migrations...)
	last := len(edited.Migrations) - 1
	edited.Migrations[last].DownSQL += "\n-- an edit made after this was applied\n"

	reverted, err := dbmigrate.Down(ctx, conn, edited, 1)
	if err == nil {
		t.Fatal("the revert ran against a database that applied different content — it would drop " +
			"what the source names rather than what the database has, and then delete the only " +
			"record of what it applied")
	}
	if reverted != 0 {
		t.Errorf("reverted %d migration(s) before refusing, want 0", reverted)
	}
	if !strings.Contains(err.Error(), "applied content does not match the source") {
		t.Errorf("the revert was refused by something else: %v", err)
	}

	// And the UNEDITED revert still runs, so the guard refuses an edit rather
	// than refusing rollbacks.
	if _, err := dbmigrate.Down(ctx, conn, core, 1); err != nil {
		t.Errorf("the revert of the content that was applied refused: %v", err)
	}
}

// Up REFUSES to migrate past a version whose applied content is not the content
// this binary holds, and refuses before applying ANY of the namespace.
//
// The refusal is the decision #2141 asked for. An edited migration is skipped as
// done, so the edit is absent on this database and present on every fresh
// installation, and every later migration is then applied on top of a schema
// the source cannot describe — the divergence compounds a boot at a time and
// nothing says so. Warning instead would leave that silent, and the same defect
// arriving as a RENUMBER is already refused here (assertLedgerMatches), so
// warning about one and refusing the other would be two answers to one
// question.
//
// Driven through dbmigrate.Up rather than against the predicate, because the
// predicate was already right and unread: what this pins is that the production
// migrate path consults it.
func TestMigrations_upRefusesAnEditedMigrationBeforeApplyingAny(t *testing.T) {
	ownerDSN, _ := dsns(t)
	conn := connect(t, ownerDSN)
	resetSchema(t, conn)
	ctx := context.Background()

	core, err := migrations.Core()
	if err != nil {
		t.Fatalf("loading core: %v", err)
	}

	// Applied WITHOUT the newest migration, so the run under test has real work
	// to do. A namespace with nothing left to apply would refuse on an empty
	// loop and prove nothing about the ordering below.
	head := len(core.Migrations) - 1
	partial := core
	partial.Migrations = core.Migrations[:head]
	if _, err := dbmigrate.Up(ctx, conn, partial); err != nil {
		t.Fatalf("up to the penultimate migration: %v", err)
	}

	// The edit a contributor would make, to a migration this database has
	// ALREADY applied — the first one, so the pending work sits after it.
	edited := core
	edited.Migrations = append([]dbmigrate.Migration(nil), core.Migrations...)
	edited.Migrations[0].UpSQL += "\n-- an edit made after this was applied\n"

	applied, err := dbmigrate.Up(ctx, conn, edited)
	if err == nil {
		t.Fatal("the migration ran past a version whose applied content differs from the source — " +
			"the edit is absent here and present on every fresh installation, and nothing says so")
	}
	if !strings.Contains(err.Error(), "applied content does not match the source") {
		t.Fatalf("the run was refused by something else: %v", err)
	}
	// BEFORE any of them. Judged inside the apply loop, the edit on migration
	// one would be caught before anything ran; judged there with the edit on a
	// LATER version, the run would move the database somewhere new and then
	// refuse, and the operator would learn one defect per boot.
	if applied != 0 {
		t.Errorf("applied %d migration(s) before refusing, want 0 — a run that half-migrates and "+
			"then stops leaves the database at a version neither source describes as current", applied)
	}

	// The unedited run still completes, so the guard refuses an edit rather
	// than refusing migration.
	if _, err := dbmigrate.Up(ctx, conn, core); err != nil {
		t.Errorf("the run of the content that was applied refused: %v", err)
	}
}
