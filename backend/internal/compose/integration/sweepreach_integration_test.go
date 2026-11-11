// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The merge and the Art. 17 erasure reach a table because its owner declared
// it (sweepreach), not because core names it. These run the real merge and the
// real eraser over a core module's declaration and over a unit's ext table.

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/corereach"
	"github.com/margince/margince/backend/internal/modules/privacy"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/platform/sweepreach"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/pkg/extension"
	"github.com/margince/margince/backend/pkg/extension/reach"
)

// unitNote is a unit table holding a company id, a contact id and free text
// about the contact.
var unitNote = reach.Table{
	Name: "ext_reachtest_note",
	Links: []reach.Link{
		{Column: "company_id", Record: reach.Company},
		{Column: "contact_id", Record: reach.Contact},
	},
	Erase: []reach.Clear{
		{Column: "body", To: reach.Tombstone},
		{Column: "contact_id", To: reach.Null},
	},
}

// withUnitNoteTable creates the unit table and installs its declaration beside
// the core ones, undoing both when the test ends.
func withUnitNoteTable(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	owner, err := pgx.Connect(ctx, os.Getenv("MARGINCE_TEST_DSN"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := owner.Exec(ctx, `
		CREATE TABLE ext.ext_reachtest_note (
		  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
		  company_id uuid, contact_id uuid, body text NOT NULL);
		GRANT SELECT, INSERT, UPDATE, DELETE ON ext.ext_reachtest_note TO margince_app`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(), `DROP TABLE ext.ext_reachtest_note`); err != nil {
			t.Errorf("dropping the unit table: %v", err)
		}
		if err := owner.Close(context.Background()); err != nil {
			t.Errorf("closing the owner connection: %v", err)
		}
		if err := corereach.Install(nil); err != nil {
			t.Errorf("restoring the core declarations: %v", err)
		}
	})
	if err := corereach.Install([]extension.Extension{{Name: "reachtest", Reach: []reach.Table{unitNote}}}); err != nil {
		t.Fatal(err)
	}
}

// seedPlanLines writes one plan line per (kind, record) on one week's plan.
func seedPlanLines(t *testing.T, e *Env, lines map[string][]ids.UUID) {
	t.Helper()
	plan := ids.NewV7()
	e.WsExec(t, `INSERT INTO weekly_plan (id, owner_id, local_week_start, captured_by)
		VALUES ($1, $2, '2026-10-05', 'human:x')`, plan, e.Rep1)
	for kind, records := range lines {
		for _, record := range records {
			e.WsExec(t, `INSERT INTO weekly_plan_commitment (plan_id, label, captured_by, linked_record_type, linked_record_id)
				VALUES ($1, 'call them', 'human:x', $2, $3)`, plan, kind, record)
		}
	}
}

const planLinesNaming = `SELECT count(*) FROM weekly_plan_commitment WHERE linked_record_type = $1 AND linked_record_id = $2`

// The weekly plan's declared pair follows a company, a contact and a lead
// merge. A line naming a record nobody merged stays where it is.
func TestEveryMergeMovesThePlanLinesTheWeeklyPlanDeclared(t *testing.T) {
	e := Setup(t)
	admin := e.Admin()
	src, tgt := seedMergePair(t, e, "Plan lines")
	bystander := e.SeedCompany(t, "Plan lines Bystander", nil)
	retired, survivor := e.SeedContact(t, "Ida Keller", nil), e.SeedContact(t, "Ida Kellar", nil)
	loser, live := seedLead(t, e, "Ola Berg", "ola@berg.test", nil), seedLead(t, e, "Ola Bergh", "", nil)
	seedPlanLines(t, e, map[string][]ids.UUID{
		"company": {src.UUID, bystander}, "contact": {retired}, "lead": {loser.UUID},
	})

	if _, err := e.Contacts.MergeCompany(admin, src, tgt, nil); err != nil {
		t.Fatalf("company merge: %v", err)
	}
	if _, err := e.Contacts.MergeContact(admin, ContactIDOf(retired), ContactIDOf(survivor), nil); err != nil {
		t.Fatalf("contact merge: %v", err)
	}
	if _, err := e.Contacts.MergeLead(admin, loser, live); err != nil {
		t.Fatalf("lead merge: %v", err)
	}
	for _, c := range []struct {
		kind           string
		gone, followed ids.UUID
	}{
		{"company", src.UUID, tgt.UUID},
		{"contact", retired, survivor},
		{"lead", loser.UUID, live.UUID},
	} {
		if n := e.WsCount(t, planLinesNaming, c.kind, c.gone); n != 0 {
			t.Errorf("%d plan lines still name the retired %s", n, c.kind)
		}
		if n := e.WsCount(t, planLinesNaming, c.kind, c.followed); n != 1 {
			t.Errorf("the surviving %s holds %d plan lines, want 1", c.kind, n)
		}
	}
	if n := e.WsCount(t, planLinesNaming, "company", bystander); n != 1 {
		t.Errorf("the bystander company holds %d plan lines, want its own 1", n)
	}
}

// A unit's declared company column follows the company merge; a row naming a
// third company is left alone.
func TestACompanyMergeMovesAUnitsDeclaredColumn(t *testing.T) {
	e := Setup(t)
	withUnitNoteTable(t)
	src, tgt := seedMergePair(t, e, "Unit notes")
	bystander := e.SeedCompany(t, "Unit notes Bystander", nil)
	e.WsExec(t, `INSERT INTO ext.ext_reachtest_note (company_id, body) VALUES ($1, 'a'), ($2, 'b')`, src, bystander)

	if _, err := e.Contacts.MergeCompany(e.Admin(), src, tgt, nil); err != nil {
		t.Fatalf("merge: %v", err)
	}
	for company, want := range map[ids.UUID]int{src.UUID: 0, tgt.UUID: 1, bystander: 1} {
		if n := e.WsCount(t, `SELECT count(*) FROM ext.ext_reachtest_note WHERE company_id = $1`, company); n != want {
			t.Errorf("company %s holds %d unit notes, want %d", company, n, want)
		}
	}
}

// The declared relink runs in the caller's transaction, so a merge that rolls
// back leaves the unit's rows where they were.
func TestADeclaredRelinkRollsBackWithItsTransaction(t *testing.T) {
	e := Setup(t)
	withUnitNoteTable(t)
	src, tgt := seedMergePair(t, e, "Rolled back")
	e.WsExec(t, `INSERT INTO ext.ext_reachtest_note (company_id, body) VALUES ($1, 'a')`, src)

	abort := errors.New("abort the merge")
	ctx := principal.WithWorkspaceID(context.Background(), e.WS)
	err := database.WithWorkspaceTx(ctx, e.Pool, func(tx pgx.Tx) error {
		if err := sweepreach.Relink(ctx, tx, reach.Company, src.UUID, tgt.UUID); err != nil {
			return err
		}
		return abort
	})
	if !errors.Is(err, abort) {
		t.Fatalf("transaction ended with %v, want the abort", err)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM ext.ext_reachtest_note WHERE company_id = $1`, src); n != 1 {
		t.Errorf("the retired company holds %d unit notes after the rollback, want 1", n)
	}
}

// The erasure clears what a unit's table says about the subject, leaves a row
// about somebody else, and tombstones each row it cleared.
func TestTheErasureClearsAUnitsDeclaredColumns(t *testing.T) {
	e := Setup(t)
	withUnitNoteTable(t)
	subject, other := e.SeedContact(t, "Anna Weber", nil), e.SeedContact(t, "Ben Ott", nil)
	named, kept := ids.NewV7(), ids.NewV7()
	e.WsExec(t, `INSERT INTO ext.ext_reachtest_note (id, contact_id, body) VALUES ($1, $2, 'Anna wants a call'), ($3, $4, 'Ben wants a call')`,
		named, subject, kept, other)

	if err := privacy.NewEraser(e.DB()).EraseContact(e.Admin(), subject, "subject request"); err != nil {
		t.Fatalf("erase: %v", err)
	}
	if got := e.WsScalar(t, `SELECT body || '|' || coalesce(contact_id::text, '') FROM ext.ext_reachtest_note WHERE id = $1`, named); got != "(erased: the contact this named exercised erasure)|" {
		t.Errorf("the subject's note reads %q, want the tombstone and no contact", got)
	}
	if got := e.WsScalar(t, `SELECT body FROM ext.ext_reachtest_note WHERE id = $1`, kept); got != "Ben wants a call" {
		t.Errorf("another contact's note reads %q, want it untouched", got)
	}
	if n := e.WsCount(t, `SELECT count(*) FROM audit_log WHERE entity_type = 'ext_reachtest_note' AND entity_id = $1 AND action = 'erase'`, named); n != 1 {
		t.Errorf("the cleared note has %d erase audit rows, want 1", n)
	}
}

// The boot refuses a unit declaring reach into a table outside its own
// namespace, before anything is applied.
func TestTheBootRefusesAUnitReachingATableItDoesNotOwn(t *testing.T) {
	err := compose.RegisterExtensions([]extension.Extension{{
		Name: "reachtest", Version: "1", Description: "Declares reach into a core table.",
		Reach: []reach.Table{{Name: "finance_invoice", Links: []reach.Link{{Column: "company_id", Record: reach.Company}}}},
	}}, nil, nil)
	if err == nil || !strings.Contains(err.Error(), "not one of its own") {
		t.Fatalf("RegisterExtensions() = %v, want the foreign table refused", err)
	}
}
