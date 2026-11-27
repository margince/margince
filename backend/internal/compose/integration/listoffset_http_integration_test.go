// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// offsetPage is the part of a list answer the pager reads.
type offsetPage struct {
	Data []struct {
		ID string `json:"id"`
	} `json:"data"`
	Page struct {
		NextCursor *string `json:"next_cursor"`
		HasMore    bool    `json:"has_more"`
		Total      *int    `json:"total"`
	} `json:"page"`
}

func TestAnOffsetPastTheCapIsRefusedOnEveryListThatTakesOne(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Offset Probe", "admin@offset.test", "Admin")
	for _, path := range []string{
		"/v1/contacts?offset=10001", "/v1/companies?offset=10001", "/v1/leads?offset=10001",
		"/v1/leads?sort=full_name&offset=10001", "/v1/deals?offset=10001", "/v1/projects?offset=10001",
	} {
		var problem cursorProblem
		status := e.Call(t, "GET", path, nil, nil, &problem)
		if status != http.StatusUnprocessableEntity || len(problem.Details.Errors) != 1 {
			t.Errorf("GET %s = %d %+v, want 422 with one field error", path, status, problem)
			continue
		}
		if got := problem.Details.Errors[0]; got.Field != "offset" || got.Code != "out_of_range" {
			t.Errorf("GET %s field error = %+v, want field=offset code=out_of_range", path, got)
		}
	}
}

func TestAnOffsetPageOverTheWireCarriesTheTotalAndACursor(t *testing.T) {
	e := apptest.SetupApp(t)
	apptest.BootstrapWorkspaceSession(t, e, "Offset Pages", "admin@offsetpages.test", "Admin")
	var company struct {
		ID string `json:"id"`
	}
	if status := e.Call(t, "POST", "/v1/companies", map[string]any{"display_name": "Offset customer", "source": "manual"}, nil, &company); status != http.StatusCreated {
		t.Fatalf("creating a company = %d", status)
	}
	for _, name := range []string{"First delivery", "Second delivery", "Third delivery"} {
		body := map[string]any{"name": name, "company_id": company.ID, "source": "manual"}
		if status := e.Call(t, "POST", "/v1/projects", body, nil, nil); status != http.StatusCreated {
			t.Fatalf("creating project %q = %d", name, status)
		}
	}

	var skipped offsetPage
	if status := e.Call(t, "GET", "/v1/projects?offset=1&limit=1", nil, nil, &skipped); status != http.StatusOK {
		t.Fatalf("GET /v1/projects?offset=1 = %d", status)
	}
	if len(skipped.Data) != 1 || skipped.Page.Total == nil || *skipped.Page.Total != 3 || skipped.Page.NextCursor == nil {
		t.Fatalf("offset page = %+v, want one row, total 3 and a cursor", skipped)
	}

	var problem cursorProblem
	status := e.Call(t, "GET", "/v1/projects?offset=1&cursor="+*skipped.Page.NextCursor, nil, nil, &problem)
	if status != http.StatusUnprocessableEntity || len(problem.Details.Errors) != 1 ||
		problem.Details.Errors[0].Field != "offset" || problem.Details.Errors[0].Code != "offset_with_cursor" {
		t.Errorf("offset with a cursor = %d %+v, want 422 field=offset code=offset_with_cursor", status, problem)
	}

	var empty offsetPage
	if status := e.Call(t, "GET", "/v1/deals?limit=1", nil, nil, &empty); status != http.StatusOK {
		t.Fatalf("GET /v1/deals = %d", status)
	}
	if empty.Page.Total == nil || *empty.Page.Total != 0 {
		t.Errorf("the deal list's first page total = %v, want 0 sent rather than left out", empty.Page.Total)
	}
}
