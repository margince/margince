// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The jurisdiction page over a composed server: the change goes through the
// real PATCH, and the page reads back what the change put in force.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/ports/subjectrightsrules"
	"github.com/margince/margince/backend/pkg/extension/subjectrights"
)

func TestTheJurisdictionPageReadsBackAChange(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var page crmcontracts.InstallationJurisdiction
	if status := e.Call(t, http.MethodGet, "/v1/installation/jurisdiction", nil, nil, &page); status != http.StatusOK {
		t.Fatalf("GET jurisdiction → %d", status)
	}
	if page.Selected != nil || !page.Floor.Known {
		t.Fatalf("unset page = %+v, want nothing selected, the strictest compiled-in floor and GDPR deadlines", page)
	}
	wantKinds := map[crmcontracts.JurisdictionDeadlineKind]bool{
		crmcontracts.JurisdictionDeadlineKindAccess:      true,
		crmcontracts.JurisdictionDeadlineKindRectify:     true,
		crmcontracts.JurisdictionDeadlineKindErasure:     true,
		crmcontracts.JurisdictionDeadlineKindRestriction: true,
	}
	if len(page.Deadlines) != len(wantKinds) {
		t.Fatalf("deadlines = %+v, want one per request kind", page.Deadlines)
	}
	for _, deadline := range page.Deadlines {
		if !wantKinds[deadline.Kind] || deadline.Rule != "gdpr" || deadline.Period == nil || *deadline.Period != "P1M" || deadline.Hours != nil {
			t.Fatalf("deadline = %+v, want a distinct request kind with the GDPR calendar month", deadline)
		}
		delete(wantKinds, deadline.Kind)
	}

	wanted := map[subjectrights.RequestKind]bool{
		subjectrights.Access: true, subjectrights.Rectify: true,
		subjectrights.Erasure: true, subjectrights.Restriction: true,
	}
	for _, row := range page.Deadlines {
		kind := subjectrights.RequestKind(row.Kind)
		if !wanted[kind] {
			t.Fatalf("unexpected or repeated deadline kind %q", kind)
		}
		deadline, rule := subjectrightsrules.Resolve("", kind)
		if row.Rule != rule || row.Period == nil || *row.Period != deadline.Period.String() {
			t.Fatalf("deadline for %s = %+v, want %s from %s", kind, row, deadline.Period.String(), rule)
		}
		delete(wanted, kind)
	}
	if len(wanted) != 0 {
		t.Fatalf("missing request deadlines: %v", wanted)
	}

	if status := e.Call(t, http.MethodPatch, "/v1/installation/settings", AnyMap{"country": "zj"}, nil, nil); status != http.StatusUnprocessableEntity {
		t.Fatalf("PATCH country to a code with no pack → %d, want 422", status)
	}
	if status := e.Call(t, http.MethodPatch, "/v1/installation/settings", AnyMap{"country": "zk"}, nil, nil); status != http.StatusOK {
		t.Fatalf("PATCH country zk → %d, want 200", status)
	}
	if status := e.Call(t, http.MethodGet, "/v1/installation/jurisdiction", nil, nil, &page); status != http.StatusOK {
		t.Fatalf("GET jurisdiction → %d", status)
	}
	if page.Selected == nil || *page.Selected != "zk" || page.Floor.Known {
		t.Fatalf("page after the change = %+v, want zk selected with no floor of its own", page)
	}
	if page.Floor.Carried == nil || page.Floor.Carried.Keep != "P6Y" {
		t.Fatalf("carried = %+v, want the six-year floor the change weakened, carried for older mail", page.Floor.Carried)
	}
	if status := e.Call(t, http.MethodGet, "/v1/installation/jurisdiction/documents/none-such", nil, nil, nil); status != http.StatusNotFound {
		t.Fatalf("a document the selected pack does not ship → %d, want 404", status)
	}
}
