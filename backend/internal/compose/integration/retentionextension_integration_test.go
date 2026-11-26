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

// setupKeptTable creates a unit table whose rows link to a contact, a company,
// a lead and an activity, and declares it with an age column.
func setupKeptTable(t *testing.T) keptFixture {
	t.Helper()
	f := keptFixture{e: Setup(t), owner: OwnerConn(t)}
	for _, stmt := range []string{
		`CREATE SCHEMA IF NOT EXISTS ext`,
		`GRANT USAGE ON SCHEMA ext TO margince_app`,
		`CREATE TABLE ext.` + keptTable + ` (id uuid PRIMARY KEY, contact_id uuid, company_id uuid, lead_id uuid,
			activity_id uuid, body text, kept_at timestamptz NOT NULL)`,
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
			{Column: "lead_id", Record: extdecl.Lead},
			{Column: "activity_id", Record: extdecl.Activity},
		},
		Personal: []string{"body"}, Retention: jurisdiction.CommercialCorrespondence, AgeColumn: "kept_at",
	}})
	return f
}

// keptRow is one row of the probe table, aged by years.
type keptRow struct {
	contact, company, lead, activity *ids.UUID
	age                              time.Duration
}

func (f keptFixture) seed(t *testing.T, r keptRow) ids.UUID {
	t.Helper()
	id := ids.NewV7()
	if _, err := f.owner.Exec(t.Context(),
		`INSERT INTO ext.`+keptTable+` (id, contact_id, company_id, lead_id, activity_id, body, kept_at)
		 VALUES ($1, $2, $3, $4, $5, 'what they said', $6)`,
		id, r.contact, r.company, r.lead, r.activity, clocktest.Now(t).Add(-r.age)); err != nil {
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
	heldLead := SeedIDRow(t, f.owner, `INSERT INTO lead (id, full_name, company_name, status, source, captured_by)
		VALUES ($1, 'Held Lead', 'Held Lead GmbH', 'new', 'manual', 'human:x')`)
	freeContact := f.e.SeedContact(t, "Free Contact", nil)
	byContact := f.seed(t, keptRow{contact: &heldContact, age: twentyYears})
	byActivity := f.seed(t, keptRow{activity: &activity, age: twentyYears})
	byLead := f.seed(t, keptRow{lead: &heldLead, age: twentyYears})
	byOneOfTwo := f.seed(t, keptRow{contact: &freeContact, company: &heldCompany, age: twentyYears})
	seam := compose.NewLegalHoldSeam(f.e.Pool)
	for kind, id := range map[string]ids.UUID{"contact": heldContact, "company": heldCompany, "lead": heldLead} {
		if err := seam.SetLegalHold(f.e.Admin(), kind, id, true, "Litigation"); err != nil {
			t.Fatal(err)
		}
	}
	f.policy(t, "erase")

	f.pass(t)

	for name, id := range map[string]ids.UUID{
		"held contact": byContact, "activity of a held company": byActivity, "held lead": byLead,
		"free contact and a held company": byOneOfTwo,
	} {
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

// probeTables creates unit tables with a contact link, a body and an age, and
// declares them in the order given.
func probeTables(t *testing.T, owner *pgx.Conn, key string, names ...string) extprivacy.Registry {
	t.Helper()
	var declared []extdecl.Table
	for _, name := range names {
		table := pgx.Identifier{"ext", name}.Sanitize()
		for _, stmt := range []string{
			`CREATE SCHEMA IF NOT EXISTS ext`,
			`GRANT USAGE ON SCHEMA ext TO margince_app`,
			`CREATE TABLE ` + table + ` (id uuid ` + key + `, contact_id uuid, body text, kept_at timestamptz NOT NULL)`,
			`GRANT SELECT, INSERT, UPDATE, DELETE ON ` + table + ` TO margince_app`,
		} {
			if _, err := owner.Exec(t.Context(), stmt); err != nil {
				t.Fatalf("preparing %s (%s): %v", name, stmt, err)
			}
		}
		t.Cleanup(func() {
			if _, err := owner.Exec(context.Background(), `DROP TABLE IF EXISTS `+table); err != nil {
				t.Errorf("removing %s: %v", name, err)
			}
		})
		declared = append(declared, extdecl.Table{
			Name: name, Links: []extdecl.Link{{Column: "contact_id", Record: extdecl.Contact}},
			Personal: []string{"body"}, Retention: jurisdiction.CommercialCorrespondence, AgeColumn: "kept_at",
		})
	}
	return extprivacy.Declared(declared)
}

func TestOneBusyExtensionTableLeavesTheNextItsShareOfAPass(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	registry := probeTables(t, owner, "PRIMARY KEY", "ext_probe_busy", "ext_probe_quiet")
	old := clocktest.Now(t).Add(-twentyYears)
	if _, err := owner.Exec(t.Context(), `INSERT INTO ext.ext_probe_busy (id, body, kept_at)
		SELECT uuidv7(), 'busy words', $1 FROM generate_series(1, 250)`, old); err != nil {
		t.Fatal(err)
	}
	quiet := ids.NewV7()
	if _, err := owner.Exec(t.Context(), `INSERT INTO ext.ext_probe_quiet (id, body, kept_at) VALUES ($1, 'quiet words', $2)`, quiet, old); err != nil {
		t.Fatal(err)
	}
	e.WsExec(t, `INSERT INTO retention_policy (object_type, category, retain_days, action) VALUES ('extension', NULL, 30, 'anonymize')`)

	service := compose.NewRetentionServiceFor(e.DB(), nil, slog.Default()).WithExtensions(registry)
	if err := service.EvaluateInstallation(RetentionPassCtx(e.WS)); err != nil {
		t.Fatal(err)
	}

	if got := e.WsCount(t, `SELECT count(*) FROM ext.ext_probe_quiet WHERE id = $1 AND body IS NULL`, quiet); got != 1 {
		t.Error("a table with more due rows than one pass takes kept the next table from its turn")
	}
	if got := e.WsCount(t, `SELECT count(*) FROM ext.ext_probe_busy WHERE body IS NULL`); got == 0 {
		t.Error("the busy table made no progress in the pass")
	}
}

func TestAnExtensionRowActedOnTwiceUnderOneIDRollsBack(t *testing.T) {
	e := Setup(t)
	owner := OwnerConn(t)
	registry := probeTables(t, owner, "NOT NULL", "ext_probe_twin")
	twin := ids.NewV7()
	if _, err := owner.Exec(t.Context(), `INSERT INTO ext.ext_probe_twin (id, body, kept_at)
		SELECT $1, 'twin words', $2 FROM generate_series(1, 2)`, twin, clocktest.Now(t).Add(-twentyYears)); err != nil {
		t.Fatal(err)
	}
	table := registry.SweptTables()[0]
	window := extprivacy.Window{OlderThan: clocktest.Now(t)}
	for name, act := range map[string]func(context.Context, pgx.Tx, extdecl.Table, extprivacy.Window, ids.UUID) (bool, error){
		"clear": registry.ClearDue, "delete": registry.DeleteDue,
	} {
		err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
			_, err := act(e.Admin(), tx, table, window, twin)
			return err
		})
		if err == nil {
			t.Errorf("%s changed two rows under one id and committed with one certificate's worth of audit", name)
		}
	}
	if got := e.WsCount(t, `SELECT count(*) FROM ext.ext_probe_twin WHERE body = 'twin words'`); got != 2 {
		t.Errorf("%d of the two rows are untouched after the refused acts, want 2", got)
	}
}

func TestACarriedFloorKeepsOnlyTheExtensionRowsFromBeforeItsChange(t *testing.T) {
	f := setupKeptTable(t)
	now := clocktest.Now(t)
	year := 365 * 24 * time.Hour
	beforeInWindow := f.seed(t, keptRow{age: 8 * year})
	afterTheChange := f.seed(t, keptRow{age: 6*year + 180*24*time.Hour})
	pastTheWindow := f.seed(t, keptRow{age: 12 * year})
	changed, since := now.Add(-7*year), now.Add(-10*year)
	window := extprivacy.Window{OlderThan: now.Add(-6 * year), CarriedBefore: &changed, CarriedSince: &since}

	var due []ids.UUID
	if err := database.WithWorkspaceTx(f.e.Admin(), f.e.Pool, func(tx pgx.Tx) error {
		var err error
		due, err = f.registry.Due(f.e.Admin(), tx, f.registry.SweptTables()[0], window, true, 10)
		return err
	}); err != nil {
		t.Fatal(err)
	}
	got := map[ids.UUID]bool{}
	for _, id := range due {
		got[id] = true
	}
	if got[beforeInWindow] || !got[afterTheChange] || !got[pastTheWindow] {
		t.Errorf("due = %v; want the row after the change (%s) and the one past the carried window (%s), never the older row the carried floor keeps (%s)",
			due, afterTheChange, pastTheWindow, beforeInWindow)
	}
}

// A hold placed while the pass decides about a row cannot slip past it. The pass
// holds the linked records FOR SHARE, so it waits for the hold and keeps the row.
func TestARetentionPassWaitsForAHoldBeingPlacedOnALinkedRecord(t *testing.T) {
	for name, link := range map[string]func(f keptFixture, record ids.UUID) keptRow{
		"a linked contact": func(_ keptFixture, record ids.UUID) keptRow {
			return keptRow{contact: &record, age: twentyYears}
		},
		"a company the linked activity names": func(f keptFixture, record ids.UUID) keptRow {
			activity := SeedIDRow(t, f.owner, `INSERT INTO activity (id, kind, subject, occurred_at, source, captured_by)
				VALUES ($1, 'email', 'thread', now(), 'manual', 'human:x')`)
			LinkActivity(t, f.owner, activity, "company", record)
			return keptRow{activity: &activity, age: twentyYears}
		},
	} {
		t.Run(name, func(t *testing.T) {
			f := setupKeptTable(t)
			table, record := "contact", f.e.SeedContact(t, "Soon Held", nil)
			if name != "a linked contact" {
				table, record = "company", f.e.SeedCompany(t, "Soon Held GmbH", nil)
			}
			row := f.seed(t, link(f, record))
			f.policy(t, "erase")

			placing, err := FreshOwnerConn(t).Begin(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			if _, err := placing.Exec(t.Context(), `UPDATE `+table+` SET legal_hold = true WHERE id = $1`, record); err != nil {
				t.Fatal(err)
			}
			finished := make(chan error, 1)
			go func() {
				service := compose.NewRetentionServiceFor(f.e.DB(), nil, slog.Default()).WithExtensions(f.registry)
				finished <- service.EvaluateInstallation(RetentionPassCtx(f.e.WS))
			}()
			waitUntilBlockedBy(t, placing)
			if err := placing.Commit(t.Context()); err != nil {
				t.Fatal(err)
			}
			if err := <-finished; err != nil {
				t.Fatal(err)
			}
			if got := f.body(t, row); got != "what they said" {
				t.Errorf("the pass expired a row whose linked record was being held: %q", got)
			}
		})
	}
}
