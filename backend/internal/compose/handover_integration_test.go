// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package compose

import (
	"errors"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/identity"
	"github.com/margince/margince/backend/internal/modules/projects"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func handoverSeamFor(e *integration.Env) handoverSeam {
	return newHandoverSeam(bulkEngineFor(e), ProjectsStoreOver(e.DB()), identity.NewService(e.Pool))
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
// colleague named, each through its own owner-change write; another rep's deal
// stays where it is, and nothing the admin can see is left behind.
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
		{"lead", ids.UUID(leads[0].Id)}, {"contact", ids.UUID(contactRows[1].Id)},
		{"company", company}, {"deal", deal}, {"project", ids.UUID(project.Id)},
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

// A capture-private contact answers to its owner alone, so the admin neither
// counts nor moves it.
func TestAHandoverLeavesWhatTheCallerCannotSee(t *testing.T) {
	e := integration.Setup(t)
	private := seedBulkContacts(t, e, e.Rep1, 1)
	e.MakeCapturePrivate(t, "contact", ids.UUID(private[0].Id), e.Rep1)
	seam := handoverSeamFor(e)

	out, err := seam.HandOver(e.Admin(), e.Rep1, e.Rep2)
	if err != nil {
		t.Fatalf("hand over: %v", err)
	}
	if out.Moved.Contact != 0 || out.Left.Contact != 0 {
		t.Errorf("handover = %+v, want the private contact neither moved nor counted", out)
	}
	if got := ownerOf(t, e, "contact", ids.UUID(private[0].Id)); got != e.Rep1 {
		t.Errorf("the private contact moved to %s", got)
	}
}

// The receiver must be somebody the caller may make an owner, the member
// themselves is no receiver, and a caller without member administration may
// not hand anybody's work on.
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
