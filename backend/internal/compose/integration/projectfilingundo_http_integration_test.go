// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The undo over the wire: a member's session reads and undoes a project filing,
// a refusal names its rule in the problem's code, and an agent bearer never
// reaches either route.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type projectFilingDTO struct {
	Filed    bool `json:"filed"`
	Undoable bool `json:"undoable"`
	Projects []struct {
		Name string `json:"name"`
	} `json:"projects"`
	Refusal *struct {
		Code string `json:"code"`
	} `json:"refusal"`
	Undone []struct {
		ByName string `json:"by_name"`
		Reason string `json:"reason"`
	} `json:"undone"`
}

func TestAMemberUndoesAProjectFilingOverHTTPAndAnAgentCannot(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	company := createdID(t, e, "/v1/companies", AnyMap{"display_name": "Filing Account"})
	project := createdID(t, e, "/v1/projects", AnyMap{"name": "Filing Engagement", "company_id": company, "source": "manual"})
	activity := createdID(t, e, "/v1/activities", AnyMap{"kind": "note", "subject": "Filed by mistake"})
	path := "/v1/activities/" + activity + "/project-filing"

	var unfiled projectFilingDTO
	if status := e.Call(t, "GET", path, nil, nil, &unfiled); status != http.StatusOK || unfiled.Filed || unfiled.Refusal != nil {
		t.Fatalf("an unfiled activity reads as %d %+v, want 200, not filed and no refusal to explain", status, unfiled)
	}
	if status := e.Call(t, "POST", "/v1/activities/"+activity+"/relink",
		AnyMap{"entity_type": "project", "entity_id": project}, nil, nil); status != http.StatusOK {
		t.Fatalf("filing under the project → %d", status)
	}
	var filed projectFilingDTO
	if status := e.Call(t, "GET", path, nil, nil, &filed); status != http.StatusOK || !filed.Filed || !filed.Undoable ||
		len(filed.Projects) != 1 || filed.Projects[0].Name != "Filing Engagement" {
		t.Fatalf("a filed activity reads as %d %+v, want filed, undoable and naming its project", status, filed)
	}

	agent := apptest.PassportBearer(t, e, "filing agent", "read", "write")
	for _, call := range []struct{ method, path string }{{"GET", path}, {"POST", path + "/undo"}} {
		if status := e.Call(t, call.method, call.path, AnyMap{"reason": "an agent's own call"}, agent, nil); status != http.StatusUnauthorized && status != http.StatusForbidden {
			t.Errorf("an agent bearer on %s %s → %d, want it refused as a human-only route", call.method, call.path, status)
		}
	}

	var problem struct {
		Code string `json:"code"`
	}
	if status := e.Call(t, "POST", path+"/undo", AnyMap{"reason": "   "}, nil, &problem); status != http.StatusUnprocessableEntity {
		t.Errorf("an unstated reason → %d, want 422", status)
	}
	var after projectFilingDTO
	if status := e.Call(t, "POST", path+"/undo", AnyMap{"reason": "the assistant filed the wrong thread"}, nil, &after); status != http.StatusOK {
		t.Fatalf("undoing the filing → %d", status)
	}
	if after.Filed || len(after.Projects) != 0 || len(after.Undone) != 1 || after.Undone[0].Reason != "the assistant filed the wrong thread" || after.Undone[0].ByName == "" {
		t.Errorf("after the undo: %+v, want unfiled with the member's decision on record", after)
	}

	if status := e.Call(t, "POST", path+"/undo", AnyMap{"reason": "again"}, nil, &problem); status != http.StatusConflict || problem.Code != "not_filed" {
		t.Errorf("a second undo → %d %q, want 409 not_filed", status, problem.Code)
	}
}
