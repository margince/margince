// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A list's settings over the wire at their edges: an unchanged save, a team
// given and taken back, a steward handed over, an archived list that refuses a
// change, and the reasons and history a Live List's filter tree leaves behind.

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

type listSettingsWire struct {
	listWire
	TeamID    *string `json:"team_id"`
	StewardID *string `json:"steward_id"`
}

func TestAListsSettingsChangeOnlyWhereTheyCanHold(t *testing.T) {
	e, _ := listsApp(t, true)
	var list listSettingsWire
	mustCall(t, e, "POST", "/v1/lists", AnyMap{"name": "Keepers", "entity_type": "contact"}, http.StatusCreated, &list)

	var same listSettingsWire
	mustCall(t, e, "PATCH", "/v1/lists/"+list.ID, AnyMap{"version": list.Version, "name": "Keepers"}, http.StatusOK, &same)
	if same.Version != list.Version {
		t.Errorf("an unchanged save moved the version %d → %d", list.Version, same.Version)
	}
	mustCall(t, e, "PATCH", "/v1/lists/"+list.ID, AnyMap{
		"version": list.Version, "definition": AnyMap{"field": "owner_id", "op": "exists", "value": true},
	}, http.StatusUnprocessableEntity, nil)

	var team, colleague struct {
		ID string `json:"id"`
	}
	e.DescribeCompany(t)
	mustCall(t, e, "POST", "/v1/teams", AnyMap{"name": "List Desk"}, http.StatusCreated, &team)
	mustCall(t, e, "POST", "/v1/users", AnyMap{
		"email": "steward-" + ids.NewV7().String()[:8] + "@lists.test", "display_name": "Steward", "role": "rep",
	}, http.StatusCreated, &colleague)

	var teamed listSettingsWire
	mustCall(t, e, "PATCH", "/v1/lists/"+list.ID, AnyMap{
		"version": list.Version, "sharing": "team", "team_id": team.ID,
	}, http.StatusOK, &teamed)
	if teamed.TeamID == nil || *teamed.TeamID != team.ID {
		t.Errorf("team_id = %v, want %s", teamed.TeamID, team.ID)
	}
	var cleared listSettingsWire
	mustCall(t, e, "PATCH", "/v1/lists/"+list.ID, AnyMap{"version": teamed.Version, "team_id": nil}, http.StatusOK, &cleared)
	if cleared.TeamID != nil {
		t.Errorf("team_id = %v after null, want none", *cleared.TeamID)
	}

	// A steward who has not yet signed in cannot look after the list.
	var handed listSettingsWire
	mustCall(t, e, "PATCH", "/v1/lists/"+list.ID, AnyMap{"version": cleared.Version, "steward_id": colleague.ID}, http.StatusOK, &handed)
	if handed.StewardID == nil || *handed.StewardID != colleague.ID || handed.Health != "ownerless" {
		t.Errorf("handed over: steward %v, health %q; want %s and ownerless", handed.StewardID, handed.Health, colleague.ID)
	}

	mustCall(t, e, "DELETE", "/v1/lists/"+list.ID, nil, http.StatusOK, nil)
	mustCall(t, e, "DELETE", "/v1/lists/"+list.ID, nil, http.StatusNotFound, nil)
	mustCall(t, e, "PATCH", "/v1/lists/"+list.ID, AnyMap{"version": handed.Version + 1, "name": "Too late"}, http.StatusConflict, nil)
}

func TestALiveListExplainsEachBranchAndKeepsEachFilterInItsHistory(t *testing.T) {
	e, _ := listsApp(t, true)
	var named struct {
		ID string `json:"id"`
	}
	mustCall(t, e, "POST", "/v1/contacts", AnyMap{"full_name": "Branch Alpha"}, http.StatusCreated, &named)
	var live listWire
	mustCall(t, e, "POST", "/v1/lists", AnyMap{
		"name": "Both branches", "entity_type": "contact", "list_type": "dynamic",
		"definition": AnyMap{"or": []any{
			AnyMap{"field": "owner_id", "op": "exists", "value": true},
			AnyMap{"field": "owner_id", "op": "exists", "value": false},
		}},
	}, http.StatusCreated, &live)

	var why struct {
		Verdict struct {
			Join     *string  `json:"join"`
			Children []AnyMap `json:"children"`
		} `json:"clauses"`
	}
	mustCall(t, e, "GET", "/v1/lists/"+live.ID+"/members/"+named.ID+"/why", nil, http.StatusOK, &why)
	if why.Verdict.Join == nil || *why.Verdict.Join != "or" || len(why.Verdict.Children) != 2 {
		t.Errorf("verdict = %+v, want an or over both clauses", why.Verdict)
	}

	mustCall(t, e, "PATCH", "/v1/lists/"+live.ID, AnyMap{
		"version": live.Version, "definition": AnyMap{"field": "owner_id", "op": "exists", "value": false},
	}, http.StatusOK, nil)
	// No limit named asks for the default page, not a page of one.
	var history pageWire
	mustCall(t, e, "GET", "/v1/lists/"+live.ID+"/history", nil, http.StatusOK, &history)
	withFilter := 0
	for _, entry := range history.Data {
		if entry["definition"] != nil {
			withFilter++
		}
	}
	if withFilter != 2 {
		t.Errorf("%d history entries carry a filter, want the first and the changed one", withFilter)
	}
}
