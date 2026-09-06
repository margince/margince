// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The account scan's HTTP surface over the real handler stack: a read before
// any open says nobody asked, an open on a deployment with no worker settles
// on the rules' floor in the same request, force is accepted as a body, and
// an account the reader cannot open is not found.

import (
	"net/http"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type scanResponse struct {
	OrganizationID string `json:"organization_id"`
	State          string `json:"state"`
	DegradeReason  string `json:"degrade_reason"`
	FindingsDrop   int    `json:"findings_dropped"`
}

func TestAccountScanHTTPSurface(t *testing.T) {
	// Wired the way the api role wires it, with no job runner and no lane:
	// the option must still stand the surface up, on the rules' floor.
	e := apptest.SetupAppWithOptions(t, compose.WithAccountScan(nil, nil, nil))
	e.BootstrapWorkspace(t)
	orgID := createdID(t, e, "/v1/organizations", AnyMap{"display_name": "Nordlicht Logistik AG", "source": "ui"})
	path := "/v1/organizations/" + orgID + "/scan"

	var before scanResponse
	if status := e.Call(t, "GET", path, nil, nil, &before); status != http.StatusOK {
		t.Fatalf("GET before any open = %d, want 200", status)
	}
	if before.State != "never" || before.OrganizationID != orgID {
		t.Errorf("before any open = %+v, want never, for this account", before)
	}

	// The test app wires no worker: the open settles the floor in-request,
	// and says so, rather than leaving the page polling for a read nothing
	// will pick up.
	var opened scanResponse
	if status := e.Call(t, "POST", path, nil, nil, &opened); status != http.StatusOK {
		t.Fatalf("POST = %d, want 200", status)
	}
	if opened.State != "degraded" || !strings.Contains(opened.DegradeReason, "No worker") {
		t.Errorf("opened = %+v, want degraded because no worker runs scans here", opened)
	}
	var forced scanResponse
	if status := e.Call(t, "POST", path, AnyMap{"force": true}, nil, &forced); status != http.StatusOK {
		t.Fatalf("POST force = %d, want 200", status)
	}
	if forced.State != "degraded" {
		t.Errorf("forced = %+v, want the floor again", forced)
	}
	var after scanResponse
	if status := e.Call(t, "GET", path, nil, nil, &after); status != http.StatusOK || after.State != "degraded" {
		t.Errorf("GET after the open = %d %+v, want the settled floor", status, after)
	}

	unknown := "/v1/organizations/00000000-0000-7000-8000-000000000000/scan"
	if status := e.Call(t, "GET", unknown, nil, nil, nil); status != http.StatusNotFound {
		t.Errorf("GET on an account that is not there = %d, want 404", status)
	}
	if status := e.Call(t, "POST", unknown, nil, nil, nil); status != http.StatusNotFound {
		t.Errorf("POST on an account that is not there = %d, want 404", status)
	}
}

// The attention and worklist surfaces are forwarded by hand from the Server
// to their assembled handlers, one method per operation. Each answers over
// HTTP, so a forward that was dropped or crossed fails here rather than in
// a rep's morning.
func TestAttentionSurfacesAnswerOverHTTP(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	for _, path := range []string{
		"/v1/attention", "/v1/magic", "/v1/worklist", "/v1/worklist/team", "/v1/worklist/handled",
		"/v1/worklist/exceptions", "/v1/worklist/hidden", "/v1/worklist/response",
	} {
		if status := e.Call(t, "GET", path, nil, nil, nil); status != http.StatusOK {
			t.Errorf("GET %s = %d, want 200", path, status)
		}
	}
}
