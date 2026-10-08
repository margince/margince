// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// Every /v1/lists route over the wire, as the web client drives it: a Live
// List and a Shortlist made, read, changed, archived and restored; members
// added, paged, explained and removed; the history read; and the record
// lists, the company page and the export each narrowed to a list.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type listWire struct {
	ID           string `json:"id"`
	Version      int64  `json:"version"`
	Purpose      *string
	Sharing      string `json:"sharing"`
	VisibleCount *int   `json:"visible_count"`
	Health       string `json:"health"`
	CanEdit      bool   `json:"can_edit"`
	ArchivedAt   *string
	Dependencies []AnyMap `json:"dependencies"`
}

type pageWire struct {
	Data []AnyMap `json:"data"`
	Page struct {
		HasMore    bool    `json:"has_more"`
		NextCursor *string `json:"next_cursor"`
	} `json:"page"`
}

// mustCall fails the test unless the call answers want.
//
//craft:ignore naked-any body and out are the request and answer shapes each route takes
func mustCall(t *testing.T, e *apptest.AppEnv, method, path string, body any, want int, out any) {
	t.Helper()
	if status := e.Call(t, method, path, body, nil, out); status != want {
		t.Fatalf("%s %s → %d, want %d", method, path, status, want)
	}
}

func TestEveryListRouteAnswersOverTheWire(t *testing.T) {
	e, _ := listsApp(t, true)
	var a, b, company AnyMap
	mustCall(t, e, "POST", "/v1/contacts", AnyMap{"source": "manual", "full_name": "Route Alpha"}, http.StatusCreated, &a)
	mustCall(t, e, "POST", "/v1/contacts", AnyMap{"source": "manual", "full_name": "Route Beta"}, http.StatusCreated, &b)
	mustCall(t, e, "POST", "/v1/companies", AnyMap{"display_name": "Route Account", "source": "manual"}, http.StatusCreated, &company)

	var live, short listWire
	mustCall(t, e, "POST", "/v1/lists", AnyMap{
		"name": "Everyone named Route", "entity_type": "contact", "list_type": "dynamic",
		"definition": AnyMap{"field": "owner_id", "op": "exists", "value": true},
		"purpose":    "route test", "sharing": "workspace",
	}, http.StatusCreated, &live)
	mustCall(t, e, "POST", "/v1/lists", AnyMap{"name": "Chosen", "entity_type": "contact"}, http.StatusCreated, &short)

	// A Shortlist takes members by hand; a Live List refuses them.
	for _, c := range []AnyMap{a, b} {
		mustCall(t, e, "POST", "/v1/lists/"+short.ID+"/members", AnyMap{
			"entity_type": "contact", "entity_id": c["id"], "note": "picked",
		}, http.StatusCreated, nil)
	}
	mustCall(t, e, "POST", "/v1/lists/"+live.ID+"/members", AnyMap{
		"entity_type": "contact", "entity_id": a["id"],
	}, http.StatusUnprocessableEntity, nil)
	mustCall(t, e, "POST", "/v1/lists/"+short.ID+"/members", AnyMap{
		"entity_type": "contact", "entity_id": a["id"],
	}, http.StatusConflict, nil)
	mustCall(t, e, "POST", "/v1/lists/"+short.ID+"/members", AnyMap{
		"entity_type": "company", "entity_id": company["id"],
	}, http.StatusUnprocessableEntity, nil)
	mustCall(t, e, "POST", "/v1/lists/"+ids.NewV7().String()+"/members/remove", AnyMap{
		"entity_type": "contact", "entity_id": a["id"],
	}, http.StatusNotFound, nil)

	// Members page by page, of both kinds.
	for _, id := range []string{short.ID, live.ID} {
		var first, second pageWire
		mustCall(t, e, "GET", "/v1/lists/"+id+"/members?limit=1", nil, http.StatusOK, &first)
		if len(first.Data) != 1 || !first.Page.HasMore || first.Page.NextCursor == nil {
			t.Fatalf("list %s first page = %+v, want one member and a cursor", id, first)
		}
		mustCall(t, e, "GET", "/v1/lists/"+id+"/members?limit=1&cursor="+*first.Page.NextCursor, nil, http.StatusOK, &second)
		if len(second.Data) != 1 || second.Data[0]["entity_id"] == first.Data[0]["entity_id"] {
			t.Fatalf("list %s second page = %+v", id, second)
		}
	}
	mustCall(t, e, "GET", "/v1/lists/"+short.ID+"/members?cursor=not-a-cursor", nil, http.StatusUnprocessableEntity, nil)

	// Why, for both kinds.
	var liveWhy, chosenWhy AnyMap
	mustCall(t, e, "GET", "/v1/lists/"+live.ID+"/members/"+a["id"].(string)+"/why", nil, http.StatusOK, &liveWhy)
	mustCall(t, e, "GET", "/v1/lists/"+short.ID+"/members/"+b["id"].(string)+"/why", nil, http.StatusOK, &chosenWhy)
	if liveWhy["member"] != true || liveWhy["clauses"] == nil || chosenWhy["note"] != "picked" || chosenWhy["added_by_name"] == nil {
		t.Fatalf("why answered %v and %v", liveWhy, chosenWhy)
	}

	// The record list narrowed to each list.
	var narrowed pageWire
	mustCall(t, e, "GET", "/v1/contacts?list_id="+short.ID, nil, http.StatusOK, &narrowed)
	if len(narrowed.Data) != 2 {
		t.Fatalf("contacts on the Shortlist = %d, want 2", len(narrowed.Data))
	}
	mustCall(t, e, "GET", "/v1/contacts?list_id="+live.ID, nil, http.StatusOK, &narrowed)
	mustCall(t, e, "GET", "/v1/companies?list_id="+short.ID, nil, http.StatusUnprocessableEntity, nil)

	// Update, archive (read-only), restore, library filters.
	var changed listWire
	mustCall(t, e, "PATCH", "/v1/lists/"+short.ID, AnyMap{
		"version": short.Version, "purpose": "launch", "sharing": "private", "name": "Chosen few",
	}, http.StatusOK, &changed)
	mustCall(t, e, "PATCH", "/v1/lists/"+short.ID, AnyMap{"version": changed.Version, "purpose": nil}, http.StatusOK, &changed)
	mustCall(t, e, "PATCH", "/v1/lists/"+short.ID, AnyMap{"version": short.Version, "name": "stale"}, http.StatusConflict, nil)
	mustCall(t, e, "PATCH", "/v1/lists/"+live.ID, AnyMap{
		"version": live.Version, "definition": AnyMap{"field": "owner_id", "op": "exists", "value": false},
	}, http.StatusOK, nil)
	mustCall(t, e, "PATCH", "/v1/lists/"+live.ID, AnyMap{
		"version": live.Version + 1, "definition": AnyMap{"field": "no_such_field", "op": "eq", "value": "x"},
	}, http.StatusUnprocessableEntity, nil)
	mustCall(t, e, "POST", "/v1/lists/"+short.ID+"/members/remove", AnyMap{
		"entity_type": "contact", "entity_id": b["id"], "note": "declined",
	}, http.StatusNoContent, nil)
	mustCall(t, e, "DELETE", "/v1/lists/"+short.ID, nil, http.StatusOK, nil)
	mustCall(t, e, "POST", "/v1/lists/"+short.ID+"/members", AnyMap{
		"entity_type": "contact", "entity_id": b["id"],
	}, http.StatusConflict, nil)
	mustCall(t, e, "POST", "/v1/lists/"+short.ID+"/restore", nil, http.StatusOK, nil)
	mustCall(t, e, "POST", "/v1/lists/"+short.ID+"/restore", nil, http.StatusConflict, nil)

	var library struct {
		Data []listWire `json:"data"`
	}
	mustCall(t, e, "GET", "/v1/lists?entity_type=contact&list_type=static&q=Chosen&include_archived=true", nil, http.StatusOK, &library)
	if len(library.Data) != 1 || library.Data[0].ID != short.ID {
		t.Fatalf("the library filtered to Chosen = %+v", library.Data)
	}

	var history pageWire
	mustCall(t, e, "GET", "/v1/lists/"+short.ID+"/history?limit=2", nil, http.StatusOK, &history)
	if len(history.Data) != 2 || !history.Page.HasMore {
		t.Fatalf("history first page = %+v", history)
	}
	mustCall(t, e, "GET", "/v1/lists/"+short.ID+"/history?limit=50&cursor="+*history.Page.NextCursor, nil, http.StatusOK, &history)

	// A first visit, and one straight after it that only extends it, have no
	// earlier visit to count from.
	var firstVisit, secondVisit AnyMap
	mustCall(t, e, "POST", "/v1/lists/"+live.ID+"/visit", nil, http.StatusOK, &firstVisit)
	mustCall(t, e, "POST", "/v1/lists/"+live.ID+"/visit", nil, http.StatusOK, &secondVisit)
	if firstVisit["previous_visit_at"] != nil || secondVisit["previous_visit_at"] != nil || secondVisit["visited_at"] == nil {
		t.Fatalf("visits answered %v then %v", firstVisit, secondVisit)
	}

	// Both kinds export — a Shortlist its members — and the Live List is then
	// named among its uses.
	mustCall(t, e, "POST", "/v1/exports", AnyMap{"list_id": live.ID, "format": "json"}, http.StatusOK, nil)
	mustCall(t, e, "POST", "/v1/exports", AnyMap{"list_id": short.ID, "format": "json"}, http.StatusOK, nil)
	mustCall(t, e, "POST", "/v1/exports", AnyMap{"list_id": "not-a-uuid", "format": "json"}, http.StatusUnprocessableEntity, nil)
	mustCall(t, e, "POST", "/v1/exports", AnyMap{"list_id": live.ID, "object": "contact", "format": "json"}, http.StatusUnprocessableEntity, nil)
	mustCall(t, e, "POST", "/v1/exports", AnyMap{"object": "contact", "format": "json"}, http.StatusUnprocessableEntity, nil)
	var used listWire
	mustCall(t, e, "GET", "/v1/lists/"+live.ID, nil, http.StatusOK, &used)
	if len(used.Dependencies) != 1 {
		t.Fatalf("the exported list names %d uses, want 1", len(used.Dependencies))
	}
}

func TestACompanysShortlistsAndTheOtherRecordListsNarrowByList(t *testing.T) {
	e, _ := listsApp(t, true)
	var company, lead AnyMap
	mustCall(t, e, "POST", "/v1/companies", AnyMap{"display_name": "Listed Account", "source": "manual"}, http.StatusCreated, &company)
	mustCall(t, e, "POST", "/v1/leads", AnyMap{"full_name": "Listed Lead", "email": "listed@lead.test", "source": "manual"}, http.StatusCreated, &lead)
	deal := apptest.CreateOpenDeal(t, e, apptest.DiscoverSeededPipeline(t, e))

	for _, c := range []struct{ entity, id, path string }{
		{"company", company["id"].(string), "/v1/companies"},
		{"lead", lead["id"].(string), "/v1/leads"},
		{"deal", deal, "/v1/deals"},
	} {
		var list listWire
		mustCall(t, e, "POST", "/v1/lists", AnyMap{"name": "Picks " + c.entity, "entity_type": c.entity}, http.StatusCreated, &list)
		mustCall(t, e, "POST", "/v1/lists/"+list.ID+"/members", AnyMap{"entity_type": c.entity, "entity_id": c.id}, http.StatusCreated, nil)
		var page pageWire
		mustCall(t, e, "GET", c.path+"?list_id="+list.ID, nil, http.StatusOK, &page)
		if len(page.Data) != 1 || page.Data[0]["id"] != c.id {
			t.Fatalf("%s narrowed to its Shortlist = %v", c.path, page.Data)
		}
	}

	var view struct {
		ListMemberships []AnyMap `json:"list_memberships"`
	}
	mustCall(t, e, "GET", "/v1/companies/"+company["id"].(string)+"/360", nil, http.StatusOK, &view)
	if len(view.ListMemberships) != 1 || view.ListMemberships[0]["name"] != "Picks company" {
		t.Fatalf("the company page names %v, want its one Shortlist", view.ListMemberships)
	}
}

func TestTheAgentsListModesAnswerAsTheRoutesDo(t *testing.T) {
	e, agent := listsApp(t, true)
	var contact AnyMap
	mustCall(t, e, "POST", "/v1/contacts", AnyMap{"source": "manual", "full_name": "Agent Pick"}, http.StatusCreated, &contact)
	var created struct {
		Result listWire `json:"result"`
	}
	agent.CallOK(t, "change_lists", map[string]any{
		"mode": "create", "name": "Agent picks", "entity_type": "contact", "sharing": "team", "purpose": "tested",
	}).JSON(t, &created)
	id := created.Result.ID
	for _, call := range []map[string]any{
		{"mode": "add_member", "list_id": id, "entity_type": "contact", "record_id": contact["id"], "note": "why"},
		{"mode": "update", "list_id": id, "version": created.Result.Version, "name": "Agent picks, renamed"},
		{"mode": "remove_member", "list_id": id, "entity_type": "contact", "record_id": contact["id"]},
		{"mode": "archive", "list_id": id},
		{"mode": "restore", "list_id": id},
	} {
		agent.CallOK(t, "change_lists", call)
	}
	for _, call := range []map[string]any{
		{"mode": "find", "query": "Agent", "entity_type": "contact"},
		{"mode": "get", "list_id": id},
		{"mode": "members", "list_id": id, "limit": 10},
		{"mode": "why", "list_id": id, "record_id": contact["id"]},
		{"mode": "history", "list_id": id},
		{"mode": "preview", "entity_type": "contact", "definition": map[string]any{"field": "owner_id", "op": "exists", "value": true}},
	} {
		agent.CallOK(t, "read_lists", call)
	}
	agent.CallRefused(t, "read_lists", map[string]any{"mode": "preview", "entity_type": "partner", "definition": map[string]any{"field": "x", "op": "eq", "value": "y"}})
	for tool, calls := range map[string][]map[string]any{
		"read_lists": {
			{"mode": "everything"},
			{"mode": "get"},
			{"mode": "preview", "entity_type": "contact", "definition": map[string]any{"op": "eq"}},
		},
		"change_lists": {
			{"mode": "archive"},
			{"mode": "create", "name": "", "entity_type": "contact"},
			{"mode": "create", "name": "Partners", "entity_type": "partner"},
			{"mode": "create", "name": "Kinds", "entity_type": "contact", "list_type": "sometimes"},
			{"mode": "create", "name": "Shared", "entity_type": "contact", "sharing": "everybody"},
		},
	} {
		for _, call := range calls {
			agent.CallRefused(t, tool, call)
		}
	}
}
