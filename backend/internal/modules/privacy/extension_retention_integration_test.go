// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package privacy

// What the core does to the rows of an extension's tables once their age or a
// legal hold matters. A hold blocks erasure and the sweep. The sweep expires
// what is past its window and outside its floor. The core writes every audit
// row and event.

import (
	"context"
	"errors"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/blobstore"
	"github.com/margince/margince/backend/internal/platform/extprivacy"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
	extdecl "github.com/margince/margince/backend/pkg/extension/privacy"
)

const (
	agedTable  = "ext_probe_aged"
	plainTable = "ext_probe_plain"
)

// declareAgedTables creates two unit tables and declares them. The aged one
// holds personal data, so an expiry clears it. The plain one holds none, so an
// expiry deletes the row. Both count age from created_at and keep their rows
// under the commercial correspondence class.
func declareAgedTables(t *testing.T, env *sarIdentifierEnv) extprivacy.Registry {
	t.Helper()
	ctx := context.Background()
	for _, stmt := range []string{
		`CREATE SCHEMA IF NOT EXISTS ext`,
		`GRANT USAGE ON SCHEMA ext TO margince_app`,
		`CREATE TABLE ext.` + agedTable + ` (
			id uuid PRIMARY KEY DEFAULT gen_random_uuid(), contact_id uuid,
			body text NOT NULL, created_at timestamptz NOT NULL DEFAULT now())`,
		`CREATE TABLE ext.` + plainTable + ` (
			id uuid PRIMARY KEY DEFAULT gen_random_uuid(), contact_id uuid,
			created_at timestamptz NOT NULL DEFAULT now())`,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ext.` + agedTable + `, ext.` + plainTable + ` TO margince_app`,
	} {
		if _, err := env.owner.Exec(ctx, stmt); err != nil {
			t.Fatalf("preparing the unit tables (%s): %v", stmt, err)
		}
	}
	t.Cleanup(func() {
		if _, err := env.owner.Exec(context.Background(), `DROP TABLE ext.`+agedTable+`, ext.`+plainTable); err != nil {
			t.Errorf("removing the unit tables: %v", err)
		}
	})
	link := []extdecl.Link{{Column: "contact_id", Record: extdecl.Contact}}
	return extprivacy.New([]extdecl.Table{
		{Name: agedTable, Links: link, Personal: []string{"body"}, Retention: jurisdiction.CommercialCorrespondence, AgeColumn: "created_at"},
		{Name: plainTable, Links: link, Retention: jurisdiction.CommercialCorrespondence, AgeColumn: "created_at"},
	})
}

// seedAgedRow inserts one row aged the given number of days and returns its id.
func seedAgedRow(t *testing.T, env *sarIdentifierEnv, table string, days int) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	query := `INSERT INTO ext.` + table + ` (id, contact_id, created_at) VALUES ($1, $2, now() - make_interval(days => $3))`
	if table == agedTable {
		query = `INSERT INTO ext.` + table + ` (id, contact_id, body, created_at) VALUES ($1, $2, 'private words', now() - make_interval(days => $3))`
	}
	if _, err := env.owner.Exec(context.Background(), query, id, env.contact.UUID, days); err != nil {
		t.Fatal(err)
	}
	return id
}

func extensionRetention(env *sarIdentifierEnv, registry extprivacy.Registry) *RetentionService {
	noPurge := func(context.Context, pgx.Tx, []ids.UUID) error { return nil }
	return NewRetentionService(env.db, blobstore.NewMemory(), slog.New(slog.DiscardHandler), noPurge).WithExtensions(registry)
}

func enableExpiry(t *testing.T, env *sarIdentifierEnv, table string) {
	t.Helper()
	if _, err := env.owner.Exec(context.Background(),
		`INSERT INTO retention_policy (object_type, category, retain_days, action) VALUES ('extension', $1, 30, 'expire')`,
		table); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := env.owner.Exec(context.Background(),
			`DELETE FROM retention_policy WHERE object_type = 'extension' AND category = $1`, table); err != nil {
			t.Errorf("removing the policy for %s: %v", table, err)
		}
	})
}

// rowExists asks about a row of the plain table, whose rows an expiry deletes.
func rowExists(t *testing.T, env *sarIdentifierEnv, id ids.UUID) bool {
	t.Helper()
	var n int
	if err := env.owner.QueryRow(context.Background(), `SELECT count(*) FROM ext.`+plainTable+` WHERE id = $1`, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n == 1
}

func auditCount(t *testing.T, env *sarIdentifierEnv, action, table string, id ids.UUID) int {
	t.Helper()
	var n int
	if err := env.owner.QueryRow(context.Background(),
		`SELECT count(*) FROM audit_log WHERE action = $1 AND entity_type = $2 AND entity_id = $3`, action, table, id).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestAHoldOnAnExtensionRowIsAuditedAndRefusesANoOp(t *testing.T) {
	env := setupSARIdentifiers(t)
	registry := declareAgedTables(t, env)
	row := seedAgedRow(t, env, agedTable, 1)
	h := Handlers{db: env.db, eraser: NewEraser(env.db)}.WithExtensions(registry)

	if err := h.setExtensionHold(env.ctx, agedTable, row, true, "litigation"); err != nil {
		t.Fatal(err)
	}
	if err := h.setExtensionHold(env.ctx, agedTable, row, true, "again"); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("placing a second hold = %v, want a conflict", err)
	}
	if err := h.setExtensionHold(env.ctx, agedTable, row, false, "settled"); err != nil {
		t.Fatal(err)
	}
	if err := h.setExtensionHold(env.ctx, agedTable, row, false, "again"); !errors.Is(err, apperrors.ErrConflict) {
		t.Errorf("lifting an unheld row = %v, want a conflict", err)
	}
	if err := h.setExtensionHold(env.ctx, "ext_nobody_note", row, true, "x"); !errors.Is(err, apperrors.ErrInvalidArgument) {
		t.Errorf("a hold on a table no unit declared = %v, want an invalid argument", err)
	}
	if err := h.setExtensionHold(env.ctx, agedTable, ids.NewV7(), true, "x"); !errors.Is(err, apperrors.ErrNotFound) {
		t.Errorf("a hold on a row that does not exist = %v, want not found", err)
	}
	for _, action := range []string{"place_legal_hold", "lift_legal_hold"} {
		if n := auditCount(t, env, action, agedTable, row); n != 1 {
			t.Errorf("core wrote %d %s audit rows, want 1", n, action)
		}
	}
}

func TestErasureRefusesASubjectWhoseExtensionRowIsHeld(t *testing.T) {
	env := setupSARIdentifiers(t)
	registry := declareAgedTables(t, env)
	row := seedAgedRow(t, env, agedTable, 1)
	h := Handlers{db: env.db, eraser: NewEraser(env.db)}.WithExtensions(registry)
	if err := h.setExtensionHold(env.ctx, agedTable, row, true, "litigation"); err != nil {
		t.Fatal(err)
	}
	eraser := NewEraser(env.db).WithExtensions(registry)

	if err := eraser.EraseContact(env.ctx, env.contact.UUID, "request"); !errors.Is(err, apperrors.ErrConflict) {
		t.Fatalf("erasing a subject with a held extension row = %v, want a conflict", err)
	}
	var name, body string
	if err := env.owner.QueryRow(context.Background(), `SELECT full_name FROM contact WHERE id = $1`, env.contact).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if err := env.owner.QueryRow(context.Background(), `SELECT body FROM ext.`+agedTable+` WHERE id = $1`, row).Scan(&body); err != nil {
		t.Fatal(err)
	}
	if name != "Sara Subject" || body != "private words" {
		t.Errorf("a refused erasure still changed data: contact %q, row %q", name, body)
	}

	if err := h.setExtensionHold(env.ctx, agedTable, row, false, "settled"); err != nil {
		t.Fatal(err)
	}
	if err := eraser.EraseContact(env.ctx, env.contact.UUID, "request"); err != nil {
		t.Errorf("erasing after the hold was lifted: %v", err)
	}
}

func TestTheSweepExpiresWhatIsPastItsWindowAndSkipsAHeldRow(t *testing.T) {
	env := setupSARIdentifiers(t)
	registry := declareAgedTables(t, env)
	// Past the six-year class floor, so only the window and the hold decide.
	const old = 365 * 8
	oldPersonal, heldPersonal, youngPersonal := seedAgedRow(t, env, agedTable, old), seedAgedRow(t, env, agedTable, old), seedAgedRow(t, env, agedTable, 1)
	oldPlain, heldPlain := seedAgedRow(t, env, plainTable, old), seedAgedRow(t, env, plainTable, old)
	enableExpiry(t, env, agedTable)
	enableExpiry(t, env, plainTable)
	h := Handlers{db: env.db, eraser: NewEraser(env.db)}.WithExtensions(registry)
	for table, id := range map[string]ids.UUID{agedTable: heldPersonal, plainTable: heldPlain} {
		if err := h.setExtensionHold(env.ctx, table, id, true, "litigation"); err != nil {
			t.Fatal(err)
		}
	}

	service := extensionRetention(env, registry)
	for range 2 {
		if err := service.EvaluateInstallation(env.ctx); err != nil {
			t.Fatal(err)
		}
	}

	if rowExists(t, env, oldPlain) {
		t.Error("an expired row of a table with no personal column was kept")
	}
	var body string
	if err := env.owner.QueryRow(context.Background(), `SELECT body FROM ext.`+agedTable+` WHERE id = $1`, oldPersonal).Scan(&body); err != nil || body != "" {
		t.Errorf("an expired row of a table with personal data: body %q, err %v; want it kept and cleared", body, err)
	}
	for _, kept := range []struct {
		table string
		id    ids.UUID
	}{{agedTable, heldPersonal}, {plainTable, heldPlain}, {agedTable, youngPersonal}} {
		if n := auditCount(t, env, "expire", kept.table, kept.id); n != 0 {
			t.Errorf("row %s of %s was expired though it is held or inside its window", kept.id, kept.table)
		}
	}
	if !rowExists(t, env, heldPlain) {
		t.Error("the sweep deleted a held row")
	}
	for table, id := range map[string]ids.UUID{agedTable: oldPersonal, plainTable: oldPlain} {
		if n := auditCount(t, env, "expire", table, id); n != 1 {
			t.Errorf("core wrote %d expire audit rows for %s across two passes, want exactly 1", n, table)
		}
	}
	var events int
	if err := env.owner.QueryRow(context.Background(), `
		SELECT count(*) FROM event_outbox WHERE envelope->>'type' = 'retention.applied' AND envelope->'payload'->>'action' = 'expire'
		   AND envelope->'entity'->>'id' = ANY($1)`, []string{oldPersonal.String(), oldPlain.String()}).Scan(&events); err != nil {
		t.Fatal(err)
	}
	if events != 2 {
		t.Errorf("%d retention.applied events with action expire, want 2", events)
	}
}

func TestTheSweepKeepsARowThatNamesAHeldContact(t *testing.T) {
	env := setupSARIdentifiers(t)
	registry := declareAgedTables(t, env)
	row := seedAgedRow(t, env, plainTable, 365*8)
	enableExpiry(t, env, plainTable)
	if _, err := env.owner.Exec(context.Background(), `UPDATE contact SET legal_hold = true WHERE id = $1`, env.contact); err != nil {
		t.Fatal(err)
	}

	if err := extensionRetention(env, registry).EvaluateInstallation(env.ctx); err != nil {
		t.Fatal(err)
	}

	if !rowExists(t, env, row) {
		t.Error("the sweep deleted a row that names a contact under legal hold")
	}
}

func TestTheSweepKeepsARowYoungerThanItsRetentionClass(t *testing.T) {
	env := setupSARIdentifiers(t)
	registry := declareAgedTables(t, env)
	inFloor, pastFloor := seedAgedRow(t, env, plainTable, 365*2), seedAgedRow(t, env, plainTable, 365*8)
	enableExpiry(t, env, plainTable)

	if err := extensionRetention(env, registry).EvaluateInstallation(env.ctx); err != nil {
		t.Fatal(err)
	}

	if !rowExists(t, env, inFloor) {
		t.Error("a row inside the six-year class floor was expired by a 30-day window")
	}
	if rowExists(t, env, pastFloor) {
		t.Error("a row past the class floor and the window was kept")
	}
}

func TestARetentionAnonymizeOfAContactRollsBackWhileItsExtensionRowIsHeld(t *testing.T) {
	env := setupSARIdentifiers(t)
	registry := declareAgedTables(t, env)
	row := seedAgedRow(t, env, agedTable, 1)
	h := Handlers{db: env.db, eraser: NewEraser(env.db)}.WithExtensions(registry)
	if err := h.setExtensionHold(env.ctx, agedTable, row, true, "litigation"); err != nil {
		t.Fatal(err)
	}

	service := extensionRetention(env, registry)
	policy := retentionPolicy{ObjectType: "contact", Action: actionAnonymize}

	err := service.applyAction(env.ctx, policy, env.contact.UUID)

	if !errors.Is(err, errExtensionRowHeld) {
		t.Fatalf("anonymizing a contact whose extension row is held = %v, want the hold refusal", err)
	}
	if err := service.apply(env.ctx, policy, env.contact.UUID); err != nil {
		t.Errorf("the pass failed on a held record instead of keeping it: %v", err)
	}
	var name, body string
	if err := env.owner.QueryRow(context.Background(), `SELECT full_name FROM contact WHERE id = $1`, env.contact).Scan(&name); err != nil {
		t.Fatal(err)
	}
	if err := env.owner.QueryRow(context.Background(), `SELECT body FROM ext.`+agedTable+` WHERE id = $1`, row).Scan(&body); err != nil {
		t.Fatal(err)
	}
	if name != "Sara Subject" || body != "private words" {
		t.Errorf("a refused anonymize left changes behind: contact %q, row %q", name, body)
	}
}

func TestASweepSkipsAPolicyForATableItCannotCountTheAgeOf(t *testing.T) {
	env := setupSARIdentifiers(t)
	registry := extprivacy.New([]extdecl.Table{
		{Name: agedTable, Retention: jurisdiction.CommercialCorrespondence},
	})
	declareAgedTables(t, env)
	row := seedAgedRow(t, env, agedTable, 365*8)
	enableExpiry(t, env, agedTable)
	enableExpiry(t, env, "ext_nobody_note")

	if err := extensionRetention(env, registry).EvaluateInstallation(env.ctx); err != nil {
		t.Fatalf("a policy the sweep cannot act on must not fail the pass: %v", err)
	}

	if auditCount(t, env, "expire", agedTable, row) != 0 {
		t.Error("the sweep expired a row of a table that declares no age column")
	}
}

func TestExtensionRowsSupportTheExpireActionOnly(t *testing.T) {
	if !SupportsRetentionAction("extension", "expire") {
		t.Error("extension rows do not support expire")
	}
	for _, action := range []string{"archive", "anonymize", "erase"} {
		if SupportsRetentionAction("extension", action) {
			t.Errorf("extension rows support %s, which has no executor for them", action)
		}
	}
}
