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
)

func TestTheJurisdictionPageReadsBackAChange(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var page crmcontracts.InstallationJurisdiction
	if status := e.Call(t, http.MethodGet, "/v1/installation/jurisdiction", nil, nil, &page); status != http.StatusOK {
		t.Fatalf("GET jurisdiction → %d", status)
	}
	if page.Selected != nil || !page.Floor.Known || len(page.Deadlines) != 3 || page.Deadlines[0].Rule != "gdpr" {
		t.Fatalf("unset page = %+v, want nothing selected, the strictest compiled-in floor and GDPR deadlines", page)
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
