// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package customfields

// Retiring a field a Live List filters on is allowed, and says so: the retire
// names the list, and the list keeps evaluating while it reports the retired
// field its steward should replace.

import (
	"net/http"
	"slices"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type liveListsWire struct {
	Lists []struct {
		ID      string `json:"id"`
		Name    string `json:"name"`
		Sharing string `json:"sharing"`
	} `json:"lists"`
	UnseenCount int `json:"unseen_count"`
}

type listWire struct {
	ID            string   `json:"id"`
	Health        string   `json:"health"`
	RetiredFields []string `json:"retired_fields"`
	VisibleCount  *int     `json:"visible_count"`
}

func TestRetiringAFieldALiveListFiltersOnNamesTheListAndKeepsItEvaluating(t *testing.T) {
	e := apptest.SetupAppWithOptions(t,
		compose.WithSchemaPool(integration.SchemaPool(t)), compose.WithListsEnabled(true))
	e.BootstrapWorkspace(t)
	status, field, problem := createCustomField(t, e, integration.AnyMap{
		"object": "contact", "label": "Loyalty Band Retire", "type": "text", "source": "manual",
	})
	if status != http.StatusCreated {
		t.Fatalf("create field → %d %+v", status, problem)
	}
	col := field.ColumnName
	if status := e.Call(t, "POST", "/v1/contacts", integration.AnyMap{
		"full_name": "Gold Member", "source": "manual", col: "gold",
	}, nil, nil); status != http.StatusCreated {
		t.Fatalf("create contact → %d", status)
	}
	var list listWire
	if status := e.Call(t, "POST", "/v1/lists", integration.AnyMap{
		"name": "Gold band", "entity_type": "contact", "list_type": "dynamic",
		"definition": integration.AnyMap{"and": []integration.AnyMap{
			{"field": "city", "op": "exists", "value": false},
			{"field": col, "op": "eq", "value": "gold"},
		}},
	}, nil, &list); status != http.StatusCreated {
		t.Fatalf("create list → %d", status)
	}
	if list.Health != "ok" || list.RetiredFields != nil {
		t.Fatalf("a list on an active field reports %q %v, want ok and no retired fields", list.Health, list.RetiredFields)
	}

	var before liveListsWire
	if status := e.Call(t, "GET", "/v1/custom-fields/"+field.ID+"/lists", nil, nil, &before); status != http.StatusOK {
		t.Fatalf("read the field's lists → %d", status)
	}
	assertNamesOnly(t, "before the retire", before, "Gold band")

	var retired struct {
		customFieldWire
		LiveLists liveListsWire `json:"live_lists"`
	}
	if status := e.Call(t, "POST", "/v1/custom-fields/"+field.ID+"/retire", nil, nil, &retired); status != http.StatusOK {
		t.Fatalf("retire → %d", status)
	}
	if retired.Status != "retired" {
		t.Fatalf("retired field status = %q", retired.Status)
	}
	assertNamesOnly(t, "the retire response", retired.LiveLists, "Gold band")

	var after listWire
	if status := e.Call(t, "GET", "/v1/lists/"+list.ID, nil, nil, &after); status != http.StatusOK {
		t.Fatalf("read the list → %d", status)
	}
	if after.Health != "retired_field" || !slices.Equal(after.RetiredFields, []string{col}) {
		t.Fatalf("the list reports %q %v, want retired_field naming %s", after.Health, after.RetiredFields, col)
	}
	if after.VisibleCount == nil || *after.VisibleCount != 1 {
		t.Fatalf("the list counts %v members, want the one gold contact still evaluated", after.VisibleCount)
	}
}

func assertNamesOnly(t *testing.T, where string, got liveListsWire, name string) {
	t.Helper()
	if len(got.Lists) != 1 || got.Lists[0].Name != name || got.UnseenCount != 0 {
		t.Fatalf("%s names %+v, want only %q and nothing unseen", where, got, name)
	}
}
