// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func handoverSeamFor(e *integration.Env) handoverSeam {
	return newHandoverSeam(bulkEngineFor(e))
}

func ownerOf(t *testing.T, e *integration.Env, table string, id ids.UUID) ids.UUID {
	t.Helper()
	owner, err := ids.Parse(e.WsScalar(t, `SELECT owner_id::text FROM `+table+` WHERE id = $1`, id))
	if err != nil {
		t.Fatalf("read the owner of %s %s: %v", table, id, err)
	}
	return owner
}

// A departed rep's leads, contacts, company, deal and project all move to the
// colleague named, each through its own owner-change write. Another rep's deal
// stays where it is. Nothing the admin can see is left behind.
func TestAHandoverMovesEverythingADepartedColleagueOwned(t *testing.T) {
	e := integration.Setup(t)
	leads := seedBulkLeads(t, e, e.Rep1, 2)
	contactRows := seedBulkContacts(t, e, e.Rep1, 2)
	company := e.SeedCompany(t, "BAER Pharma", &e.Rep1)
	pipeline, open, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "BAER renewal", pipeline, open, &e.Rep1)
	foreignDeal := e.SeedDeal(t, "Rep3's deal", pipeline, open, &e.Rep3)
	project, err := ProjectsStoreOver(e.DB()).CreateProject(e.Admin(), projects.CreateProjectInput{
		Name: "ERP replacement", CompanyID: ids.From[ids.CompanyKind](company),
		OwnerID: new(ids.From[ids.UserKind](e.Rep1)), Source: "manual",
	})
	if err != nil {
		t.Fatalf("seed project: %v", err)
	}
	deactivate(t, e, e.Rep1)
	seam := handoverSeamFor(e)

	owned, err := seam.Owned(e.Admin(), e.Rep1)
	if err != nil {
		t.Fatalf("count what the departed rep owns: %v", err)
	}
	want := crmcontracts.HandoverCounts{Lead: 2, Contact: 2, Company: 1, Deal: 1, Project: 1}
	if owned != want {
		t.Fatalf("owned = %+v, want %+v", owned, want)
	}

	out, err := seam.HandOver(e.Admin(), e.Rep1, e.Rep2)
	if err != nil {
		t.Fatalf("hand over: %v", err)
	}
	if out.Moved != want || out.Left != (crmcontracts.HandoverCounts{}) {
		t.Fatalf("handover = %+v, want everything moved and nothing left", out)
	}
	for _, row := range []struct {
		table string
		id    ids.UUID
	}{
		{"lead", ids.UUID(leads[0].Id)},
		{"contact", ids.UUID(contactRows[1].Id)},
		{"company", company},
		{"deal", deal},
		{"project", ids.UUID(project.Id)},
	} {
		if got := ownerOf(t, e, row.table, row.id); got != e.Rep2 {
			t.Errorf("%s %s is owned by %s, want the colleague it was handed to", row.table, row.id, got)
		}
	}
	if got := ownerOf(t, e, "deal", foreignDeal); got != e.Rep3 {
		t.Errorf("another rep's deal moved to %s", got)
	}
	if n := e.WsCount(t,
		`SELECT count(*) FROM audit_log WHERE entity_type = 'deal' AND entity_id = $1
		    AND before->>'owner_id' = $2 AND after->>'owner_id' = $3`,
		deal, e.Rep1.String(), e.Rep2.String()); n != 1 {
		t.Errorf("audit rows for the deal's owner change = %d, want 1", n)
	}

	again, err := seam.HandOver(e.Admin(), e.Rep1, e.Rep2)
	if err != nil || again.Moved != (crmcontracts.HandoverCounts{}) {
		t.Errorf("a second handover → %+v, %v; want nothing left to move", again, err)
	}
}

// A capture-private contact and company of a departed rep move with the
// handover and stay owner-private, now to the colleague who took them over.
func TestAHandoverMovesPrivateRecordsAndKeepsThemPrivate(t *testing.T) {
	e := integration.Setup(t)
	private := ids.UUID(seedBulkContacts(t, e, e.Rep1, 1)[0].Id)
	e.MakeCapturePrivate(t, "contact", private, e.Rep1)
	privateCompany := e.SeedCompany(t, "Quiet GmbH", &e.Rep1)
	e.MakeCapturePrivate(t, "company", privateCompany, e.Rep1)
	deactivate(t, e, e.Rep1)
	seam := handoverSeamFor(e)

	owned, err := seam.Owned(e.Admin(), e.Rep1)
	if err != nil || owned.Contact != 1 || owned.Company != 1 {
		t.Fatalf("owned = %+v, %v; want the private contact and company counted", owned, err)
	}
	out, err := seam.HandOver(e.Admin(), e.Rep1, e.Rep2)
	if err != nil {
		t.Fatalf("hand over: %v", err)
	}
	if out.Moved.Contact != 1 || out.Moved.Company != 1 || out.Left != (crmcontracts.HandoverCounts{}) {
		t.Fatalf("handover = %+v, want both private records moved and nothing left", out)
	}
	for table, id := range map[string]ids.UUID{"contact": private, "company": privateCompany} {
		if got := ownerOf(t, e, table, id); got != e.Rep2 {
			t.Errorf("the private %s is owned by %s, want the colleague it was handed to", table, got)
		}
		if v := e.WsScalar(t, `SELECT visibility FROM `+table+` WHERE id = $1`, id); v != "owner" {
			t.Errorf("the private %s became %q, want it still owner-private", table, v)
		}
		if n := e.WsCount(t,
			`SELECT count(*) FROM audit_log WHERE entity_type = $1 AND entity_id = $2
			    AND before->>'owner_id' = $3 AND after->>'owner_id' = $4`,
			table, id, e.Rep1.String(), e.Rep2.String()); n != 1 {
			t.Errorf("audit rows for the private %s's owner change = %d, want 1", table, n)
		}
	}
}

// A colleague who still works here keeps their private records: only the work
// of somebody who left is handed over that far.
func TestAHandoverLeavesAnActiveMembersPrivateRecordsAlone(t *testing.T) {
	e := integration.Setup(t)
	private := ids.UUID(seedBulkContacts(t, e, e.Rep1, 1)[0].Id)
	e.MakeCapturePrivate(t, "contact", private, e.Rep1)
	seam := handoverSeamFor(e)

	owned, err := seam.Owned(e.Admin(), e.Rep1)
	if err != nil || owned.Contact != 0 {
		t.Fatalf("owned = %+v, %v; want an active member's private contact neither counted nor shown", owned, err)
	}
	if _, err := seam.HandOver(e.Admin(), e.Rep1, e.Rep2); err != nil {
		t.Fatalf("hand over: %v", err)
	}
	if got := ownerOf(t, e, "contact", private); got != e.Rep1 {
		t.Errorf("an active member's private contact moved to %s", got)
	}
}

// The receiver must be somebody the caller may make an owner. The member is no
// receiver of their own work. A caller without member administration hands on
// nobody's work.
func TestAHandoverRefusesWhatASingleReassignmentWouldRefuse(t *testing.T) {
	e := integration.Setup(t)
	pipeline, open, _ := integration.DealFixture(t, e)
	deal := e.SeedDeal(t, "BAER renewal", pipeline, open, &e.Rep1)
	deactivate(t, e, e.Rep3)
	seam := handoverSeamFor(e)

	_, err := seam.HandOver(e.Admin(), e.Rep1, e.Rep3)
	if _, ok := errors.AsType[*handoverReceiverError](err); !ok {
		t.Errorf("handover to a deactivated colleague → %v, want the receiver refused", err)
	}
	_, err = seam.HandOver(e.Admin(), e.Rep1, e.Rep1)
	if _, ok := errors.AsType[*handoverSameUserError](err); !ok {
		t.Errorf("handover to the member themselves → %v, want it refused", err)
	}
	rep := e.As(e.Rep2, nil, integration.RepPerms)
	if _, err := seam.HandOver(rep, e.Rep1, e.Rep2); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("handover by a rep → %v, want permission denied", err)
	}
	if got := ownerOf(t, e, "deal", deal); got != e.Rep1 {
		t.Errorf("a refused handover moved the deal to %s", got)
	}
}

// A delegated member administrator may hand on a rep's work and may not touch
// an admin's: the ceiling every member verb carries holds here too.
func TestAHandoverKeepsTheCeilingOverAnAdmin(t *testing.T) {
	e := integration.Setup(t)
	e.GrantRole(t, e.Rep3, "admin")
	delegated := e.As(e.Rep2, nil, principal.Permissions{
		RoleKeys: []string{"manager"},
		Objects: map[string]principal.ObjectGrant{
			"user_admin": {Read: true, Update: true},
			"deal":       {Read: true, Update: true},
		},
		RowScope: principal.RowScopeAll,
	})
	seam := handoverSeamFor(e)

	if _, err := seam.Owned(delegated, e.Rep3); !errors.Is(err, apperrors.ErrPermissionDenied) {
		t.Errorf("a delegated administrator reading an admin's work → %v, want permission denied", err)
	}
	if _, err := seam.Owned(delegated, e.Rep1); err != nil {
		t.Errorf("a delegated administrator reading a rep's work → %v, want it admitted", err)
	}
}

// The private handover asks the receiver question itself. A caller that reaches
// it without the seam still cannot hand records to a seat that may not own them.
func TestThePrivateHandoverRefusesAReceiverWhoCannotOwn(t *testing.T) {
	e := integration.Setup(t)
	private := ids.UUID(seedBulkContacts(t, e, e.Rep1, 1)[0].Id)
	e.MakeCapturePrivate(t, "contact", private, e.Rep1)
	deactivate(t, e, e.Rep1)
	deactivate(t, e, e.Rep3)

	err := database.WithWorkspaceTx(e.Admin(), e.Pool, func(tx pgx.Tx) error {
		_, err := contacts.HandOverOwnerPrivateTx(e.Admin(), tx,
			ids.From[ids.UserKind](e.Rep1), ids.From[ids.UserKind](e.Rep3))
		return err
	})
	if _, ok := errors.AsType[*auth.AssigneeNotAllowedError](err); !ok {
		t.Errorf("handing private records to a deactivated seat → %v, want the receiver refused", err)
	}
	if got := ownerOf(t, e, "contact", private); got != e.Rep1 {
		t.Errorf("a refused private handover moved the contact to %s", got)
	}
}
