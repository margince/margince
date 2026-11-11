// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The retention pass over extension rows. The `extension` policy acts on rows by
// their declared age. It never touches a row linked to a held record.

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/clocktest"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/extprivacy"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/pkg/extension/jurisdiction"
	extdecl "github.com/margince/margince/backend/pkg/extension/privacy"
)

const keptTable = "ext_probe_kept"

type keptFixture struct {
	e        *Env
	owner    *pgx.Conn
	registry extprivacy.Registry
}

// setupKeptTable creates a unit table whose rows link to a contact, a company
// and an activity, and declares it with an age column.
func setupKeptTable(t *testing.T) keptFixture {
	t.Helper()
	f := keptFixture{e: Setup(t), owner: OwnerConn(t)}
	for _, stmt := range []string{
		`CREATE SCHEMA IF NOT EXISTS ext`,
		`GRANT USAGE ON SCHEMA ext TO margince_app`,
		`CREATE TABLE ext.` + keptTable + ` (id uuid PRIMARY KEY, contact_id uuid, company_id uuid, activity_id uuid,
			body text, kept_at timestamptz NOT NULL)`,
		`GRANT SELECT, INSERT, UPDATE, DELETE ON ext.` + keptTable + ` TO margince_app`,
	} {
		if _, err := f.owner.Exec(t.Context(), stmt); err != nil {
			t.Fatalf("preparing %s (%s): %v", keptTable, stmt, err)
		}
	}
	t.Cleanup(func() {
		if _, err := f.owner.Exec(context.Background(), `DROP TABLE IF EXISTS ext.`+keptTable); err != nil {
			t.Errorf("removing %s: %v", keptTable, err)
		}
	})
	f.registry = extprivacy.Declared([]extdecl.Table{{
		Name: keptTable,
		Links: []extdecl.Link{
			{Column: "contact_id", Record: extdecl.Contact},
			{Column: "company_id", Record: extdecl.Company},
			{Column: "activity_id", Record: extdecl.Activity},
		},
		Personal: []string{"body"}, Retention: jurisdiction.CommercialCorrespondence, AgeColumn: "kept_at",
	}})
	return f
}

// keptRow is one row of the probe table, aged by years.
type keptRow struct {
	contact, company, activity *ids.UUID
	age                        time.Duration
}

func (f keptFixture) seed(t *testing.T, r keptRow) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := f.owner.Exec(t.Context(),
		`INSERT INTO ext.`+keptTable+` (id, contact_id, company_id, activity_id, body, kept_at) VALUES ($1, $2, $3, $4, 'what they said', $5)`,
		id, r.contact, r.company, r.activity, clocktest.Now(t).Add(-r.age)); err != nil {
		t.Fatal(err)
	}
	return id
}

func (f keptFixture) policy(t *testing.T, action string) {
	t.Helper()
	f.e.WsExec(t, `INSERT INTO retention_policy (object_type, category, retain_days, action) VALUES ('extension', NULL, 30, $1)`, action)
}

func (f keptFixture) pass(t *testing.T) {
	t.Helper()
	service := compose.NewRetentionServiceFor(f.e.DB(), nil, slog.Default()).WithExtensions(f.registry)
	if err := service.EvaluateInstallation(RetentionPassCtx(f.e.WS)); err != nil {
		t.Fatal(err)
	}
}

func (f keptFixture) body(t *testing.T, id ids.UUID) string {
	t.Helper()
	return f.e.WsScalar(t, `SELECT coalesce(body, '<cleared>') FROM ext.`+keptTable+` WHERE id = $1`, id)
}

func (f keptFixture) audits(t *testing.T, id ids.UUID, action string) int {
	t.Helper()
	return f.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = $1 AND entity_id = $2 AND action = $3`, keptTable, id, action)
}

const twentyYears = 20 * 365 * 24 * time.Hour

func TestRetentionClearsAnExtensionRowPastItsWindowOnce(t *testing.T) {
	f := setupKeptTable(t)
	contact := f.e.SeedContact(t, "Old Correspondent", nil)
	old := f.seed(t, keptRow{contact: &contact, age: twentyYears})
	unlinked := f.seed(t, keptRow{age: twentyYears})
	young := f.seed(t, keptRow{contact: &contact, age: 10 * 24 * time.Hour})
	f.policy(t, "anonymize")

	f.pass(t)
	f.pass(t)

	for name, id := range map[string]ids.UUID{"old": old, "unlinked": unlinked} {
		if got := f.body(t, id); got != "<cleared>" {
			t.Errorf("%s row kept its body %q past its window", name, got)
		}
		if got := f.audits(t, id, "anonymize"); got != 1 {
			t.Errorf("%s row has %d anonymize certificates after two passes, want 1", name, got)
		}
		if got := f.e.WsCount(t, `SELECT count(*) FROM event_outbox WHERE envelope->'entity'->>'id' = $1`, id.String()); got != 1 {
			t.Errorf("%s row queued %d events after two passes, want 1", name, got)
		}
	}
	if got := f.body(t, young); got != "what they said" {
		t.Errorf("a row inside the policy's window was cleared to %q", got)
	}
}

func TestRetentionStripsTheStoredHistoryOfAnExpiredExtensionRow(t *testing.T) {
	f := setupKeptTable(t)
	old := f.seed(t, keptRow{age: twentyYears})
	if err := database.WithWorkspaceTx(f.e.Admin(), f.e.Pool, func(tx pgx.Tx) error {
		_, err := storekit.Audit(f.e.Admin(), tx, "update", keptTable, old,
			map[string]any{"body": "an earlier draft"}, map[string]any{"body": "what they said"})
		return err
	}); err != nil {
		t.Fatal(err)
	}
	f.policy(t, "anonymize")

	f.pass(t)

	if got := f.e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = $1 AND entity_id = $2 AND (after::text LIKE '%what they said%' OR before::text LIKE '%an earlier draft%')`, keptTable, old); got != 0 {
		t.Errorf("%d stored audit images still hold the expired row's body", got)
	}
}

func TestRetentionEraseDeletesAnExtensionRowPastItsWindow(t *testing.T) {
	f := setupKeptTable(t)
	old := f.seed(t, keptRow{age: twentyYears})
	f.policy(t, "erase")

	f.pass(t)

	if got := f.e.WsCount(t, `SELECT count(*) FROM ext.`+keptTable+` WHERE id = $1`, old); got != 0 {
		t.Error("the erase policy left the expired row standing")
	}
	if got := f.audits(t, old, "erase"); got != 1 {
		t.Errorf("the deleted row has %d erase certificates, want 1", got)
	}
}

func TestRetentionKeepsAnExtensionRowLinkedToAHeldRecord(t *testing.T) {
	f := setupKeptTable(t)
	heldContact := f.e.SeedContact(t, "Held Contact", nil)
	heldCompany := f.e.SeedCompany(t, "Held Company", nil)
	activity := SeedIDRow(t, f.owner, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
		VALUES ($1, 'email', 'held thread', now(), 'manual', 'human:x')`)
	LinkActivity(t, f.owner, activity, "company", heldCompany)
	byContact := f.seed(t, keptRow{contact: &heldContact, age: twentyYears})
	byActivity := f.seed(t, keptRow{activity: &activity, age: twentyYears})
	seam := compose.NewLegalHoldSeam(f.e.Pool)
	if err := seam.SetLegalHold(f.e.Admin(), "contact", heldContact, true, "Litigation"); err != nil {
		t.Fatal(err)
	}
	if err := seam.SetLegalHold(f.e.Admin(), "company", heldCompany, true, "Litigation"); err != nil {
		t.Fatal(err)
	}
	f.policy(t, "erase")

	f.pass(t)

	for name, id := range map[string]ids.UUID{"held contact": byContact, "activity of a held company": byActivity} {
		if got := f.body(t, id); got != "what they said" {
			t.Errorf("the row linked to a %s was expired to %q", name, got)
		}
	}
}

func TestErasureKeepsAnExtensionRowThatAlsoLinksAHeldRecord(t *testing.T) {
	f := setupKeptTable(t)
	subject := f.e.SeedContact(t, "Erased Subject Candidate", nil)
	heldCompany := f.e.SeedCompany(t, "Held Company", nil)
	shared := f.seed(t, keptRow{contact: &subject, company: &heldCompany, age: time.Hour})
	only := f.seed(t, keptRow{contact: &subject, age: time.Hour})
	if err := compose.NewLegalHoldSeam(f.e.Pool).SetLegalHold(f.e.Admin(), "company", heldCompany, true, "Litigation"); err != nil {
		t.Fatal(err)
	}

	eraser := privacy.NewEraser(f.e.DB()).WithRawCapturePurger(compose.RawCapturePurgerFor(f.e.DB())).WithExtensions(f.registry)
	if err := eraser.EraseContact(f.e.Admin(), subject, "Subject requested erasure"); err != nil {
		t.Fatal(err)
	}

	if got := f.body(t, shared); got != "what they said" {
		t.Errorf("erasure cleared a row that also links a held company: %q", got)
	}
	if got := f.body(t, only); got != "<cleared>" {
		t.Errorf("erasure kept the subject's own row: %q", got)
	}
}
