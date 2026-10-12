// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Lists over the wire: absent when an operator has switched them off, and,
// when on, the same answer to a user and to the agent acting for them.

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/agents"
	"github.com/margince/margince/backend/internal/modules/collections"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

func listsApp(t *testing.T, on bool) (*apptest.AppEnv, *apptest.MCPClient) {
	t.Helper()
	e := apptest.SetupAppWithOriginOptions(t, func(origin string) []compose.Option {
		return []compose.Option{
			compose.WithMCPConnector(), compose.WithMCPResource(origin + "/mcp"), compose.WithListsEnabled(on),
		}
	})
	apptest.BootstrapWorkspaceSession(t, e, "Lists", "lists-"+ids.NewV7().String()[:8]+"@fable.test", "Admin")
	return e, apptest.NewMCPClient(e, apptest.MCPBearerToken(t, e, "list agent", "read", "write"))
}

func TestListsAreAbsentWhenAnOperatorSwitchesThemOff(t *testing.T) {
	e, agent := listsApp(t, false)
	someList := ids.NewV7().String()
	for _, call := range []struct{ method, path string }{
		{"GET", "/v1/lists"},
		{"POST", "/v1/lists"},
		{"GET", "/v1/lists/" + someList},
		{"PATCH", "/v1/lists/" + someList},
		{"DELETE", "/v1/lists/" + someList},
		{"POST", "/v1/lists/" + someList + "/restore"},
		{"GET", "/v1/lists/" + someList + "/members"},
		{"POST", "/v1/lists/" + someList + "/members"},
		{"POST", "/v1/lists/" + someList + "/members/remove"},
		{"POST", "/v1/lists/" + someList + "/members/restore"},
		{"GET", "/v1/lists/" + someList + "/members/" + someList + "/why"},
		{"GET", "/v1/lists/" + someList + "/history"},
		{"GET", "/v1/contacts?list_id=" + someList},
		{"GET", "/v1/companies?list_id=" + someList},
		{"GET", "/v1/leads?list_id=" + someList},
		{"GET", "/v1/deals?list_id=" + someList},
	} {
		if status := e.Call(t, call.method, call.path, AnyMap{"name": "x", "entity_type": "contact"}, nil, nil); status != http.StatusNotFound {
			t.Errorf("%s %s = %d, want 404 while lists are off", call.method, call.path, status)
		}
	}
	var me struct {
		SettingsAvailability struct {
			Lists *bool `json:"lists"`
		} `json:"settings_availability"`
	}
	e.Call(t, "GET", "/v1/me", nil, nil, &me)
	if me.SettingsAvailability.Lists == nil || *me.SettingsAvailability.Lists {
		t.Errorf("/me says lists = %v, want false", me.SettingsAvailability.Lists)
	}
	agent.CallRefused(t, "read_lists", map[string]any{"mode": "find"})
	agent.CallRefused(t, "change_lists", map[string]any{"mode": "create", "name": "x", "entity_type": "contact"})
	if status := e.Call(t, "POST", "/v1/exports", AnyMap{"list_id": someList, "format": "json"}, nil, nil); status != http.StatusNotFound {
		t.Errorf("an export of a list = %d, want 404 while lists are off", status)
	}
	assertNoListReachesARecord(t, e, agent)
}

// assertNoListReachesARecord holds the surfaces that start from a record: the
// bulk list verbs through both doors, and the company page's Shortlists.
func assertNoListReachesARecord(t *testing.T, e *apptest.AppEnv, agent *apptest.MCPClient) {
	t.Helper()
	shortlist := seedShortlistBehindTheSwitch(t, e)
	contacts := seedBulkContacts(t, e, 1)
	for _, verb := range []string{"add_to_list", "remove_from_list"} {
		change := AnyMap{"record_type": "contact", "verb": verb, "list_id": shortlist, "items": contacts}
		if status := e.Call(t, "POST", "/v1/bulk/preview", change, nil, nil); status != http.StatusNotFound {
			t.Errorf("bulk %s = %d, want 404 while lists are off", verb, status)
		}
		change["mode"] = "preview"
		agent.CallRefused(t, "bulk_update_records", change)
	}
	var company AnyMap
	mustCall(t, e, "POST", "/v1/companies", AnyMap{"display_name": "Unlisted Account", "source": "manual"}, http.StatusCreated, &company)
	var view map[string]json.RawMessage
	mustCall(t, e, "GET", "/v1/companies/"+company["id"].(string)+"/360", nil, http.StatusOK, &view)
	if memberships, named := view["list_memberships"]; named && string(memberships) != "null" {
		t.Errorf("the company page names Shortlists %s while lists are off", memberships)
	}
}

type listDTO struct {
	ID           string `json:"id"`
	Name         string `json:"name"`
	Version      int64  `json:"version"`
	VisibleCount *int   `json:"visible_count"`
	Health       string `json:"health"`
}

func TestAUserAndTheirAgentReadOneListTheSameWay(t *testing.T) {
	e, agent := listsApp(t, true)
	var list listDTO
	if status := e.Call(t, "POST", "/v1/lists", AnyMap{
		"name": "Launch references", "entity_type": "contact", "purpose": "who we quote at launch",
	}, nil, &list); status != http.StatusCreated {
		t.Fatalf("create list → %d", status)
	}
	var member, other AnyMap
	e.Call(t, "POST", "/v1/contacts", AnyMap{"source": "manual", "full_name": "Quoted Customer"}, nil, &member)
	e.Call(t, "POST", "/v1/contacts", AnyMap{"source": "manual", "full_name": "Not Quoted"}, nil, &other)
	// The member is added through the agent's door, the read through both.
	added := agent.CallOK(t, "change_lists", map[string]any{
		"mode": "add_member", "list_id": list.ID, "entity_type": "contact",
		"record_id": member["id"], "note": "signed the quote",
	})
	assertAnswersItsSchema(t, e, "change_lists", added)

	var overHTTP listDTO
	e.Call(t, "GET", "/v1/lists/"+list.ID, nil, nil, &overHTTP)
	var answer struct {
		Mode   string          `json:"mode"`
		Result json.RawMessage `json:"result"`
	}
	read := agent.CallOK(t, "read_lists", map[string]any{"mode": "get", "list_id": list.ID})
	assertAnswersItsSchema(t, e, "read_lists", read)
	read.JSON(t, &answer)
	var overMCP listDTO
	if err := json.Unmarshal(answer.Result, &overMCP); err != nil {
		t.Fatal(err)
	}
	if overHTTP.VisibleCount == nil || *overHTTP.VisibleCount != 1 || overMCP.VisibleCount == nil ||
		*overMCP.VisibleCount != *overHTTP.VisibleCount || overMCP.Version != overHTTP.Version {
		t.Fatalf("HTTP answered %+v and the agent %+v for one list", overHTTP, overMCP)
	}

	var page struct {
		Data []AnyMap `json:"data"`
	}
	e.Call(t, "GET", "/v1/contacts?list_id="+list.ID, nil, nil, &page)
	if len(page.Data) != 1 || page.Data[0]["id"] != member["id"] {
		t.Fatalf("the contact list narrowed to the Shortlist answered %v, want only its member", page.Data)
	}
}

func TestAnAgentPreviewOfAListDefinitionIsLoggedAsARead(t *testing.T) {
	e, agent := listsApp(t, true)
	e.Call(t, "POST", "/v1/contacts", AnyMap{"source": "manual", "full_name": "Previewed Contact"}, nil, nil)
	before := countPreviews(t, e)
	agent.CallOK(t, "read_lists", map[string]any{
		"mode": "preview", "entity_type": "contact",
		"definition": map[string]any{"field": "owner_id", "op": "exists", "value": true},
	})
	if after := countPreviews(t, e); after != before+1 {
		t.Fatalf("an agent preview wrote %d system_log rows, want one", after-before)
	}
}

func countPreviews(t *testing.T, e *apptest.AppEnv) int {
	t.Helper()
	var n int
	if err := e.Owner.QueryRow(t.Context(), `SELECT count(*) FROM system_log WHERE action = 'preview'`).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

// assertAnswersItsSchema holds a served answer to the output schema its tool
// advertises, which the dispatcher only logs a miss of.
func assertAnswersItsSchema(t *testing.T, e *apptest.AppEnv, tool string, got apptest.MCPResult) {
	t.Helper()
	spec, ok := compose.NewRegistry(e.Pool, compose.SendPath{}).Spec(tool)
	if !ok {
		t.Fatalf("%s is not registered", tool)
	}
	if defect := agents.ResultDefect(spec.OutputSchema, json.RawMessage(got.Text)); defect != "" {
		t.Fatalf("%s answered outside its advertised schema: %s", tool, defect)
	}
}

// seedShortlistBehindTheSwitch writes a workspace-wide Shortlist through the
// collections store, which the switch does not gate, so a refusal proves the
// switch rather than an unknown list id.
func seedShortlistBehindTheSwitch(t *testing.T, e *apptest.AppEnv) string {
	t.Helper()
	ctx := principal.SystemActing(principal.WithWorkspaceID(t.Context(), apptest.InstallationWorkspaceUUID(t.Context(), t, e.Pool)), "system:lists-off-test")
	list, err := compose.NewCollectionsStore(e.Pool).CreateList(ctx, collections.CreateListInput{
		Name: "Hidden picks", EntityType: "contact", Sharing: "workspace",
	})
	if err != nil {
		t.Fatalf("seeding a Shortlist: %v", err)
	}
	return list.ID.String()
}
