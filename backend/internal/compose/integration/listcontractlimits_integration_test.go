// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A list write holds the limits the contract publishes, on every verb that
// carries the field.

import (
	"net/http"
	"strings"
	"testing"
)

func TestAListWriteHoldsTheContractsLimits(t *testing.T) {
	e, _ := listsApp(t, true)
	var list listDTO
	if status := e.Call(t, "POST", "/v1/lists", AnyMap{"name": "Limits", "entity_type": "contact"}, nil, &list); status != http.StatusCreated {
		t.Fatalf("create list → %d", status)
	}
	var contact AnyMap
	e.Call(t, "POST", "/v1/contacts", AnyMap{"source": "manual", "full_name": "Noted Contact"}, nil, &contact)

	members := "/v1/lists/" + list.ID + "/members"
	change := func(note string) AnyMap {
		return AnyMap{"entity_type": "contact", "entity_id": contact["id"], "note": note}
	}
	for path, note := range map[string]string{members: strings.Repeat("n", 501), members + "/remove": strings.Repeat("n", 501)} {
		if status := e.Call(t, "POST", path, change(note), nil, nil); status != http.StatusUnprocessableEntity {
			t.Errorf("POST %s with a 501-character note → %d, want 422", path, status)
		}
	}
	if status := e.Call(t, "POST", members, change(strings.Repeat("n", 500)), nil, nil); status != http.StatusCreated {
		t.Errorf("a 500-character member note → %d, want 201", status)
	}

	path := "/v1/lists/" + list.ID
	for name, body := range map[string]AnyMap{
		"a blank name":      {"version": list.Version, "name": "   "},
		"an empty name":     {"version": list.Version, "name": ""},
		"a null name":       {"version": list.Version, "name": nil},
		"no version at all": {"name": "Renamed"},
	} {
		if status := e.Call(t, "PATCH", path, body, nil, nil); status != http.StatusUnprocessableEntity {
			t.Errorf("PATCH a list with %s → %d, want 422", name, status)
		}
	}
	var after listDTO
	e.Call(t, "GET", path, nil, nil, &after)
	if after.Name != "Limits" {
		t.Errorf("a refused update left the list named %q, want it untouched", after.Name)
	}
}
