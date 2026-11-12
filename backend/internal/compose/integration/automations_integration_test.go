// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The automations surface (B-E15.1/.4, feedback/14): a closed catalog,
// instance CRUD that validates params against it, created-paused,
// If-Match on the enable flip, soft delete — and the workspace
// bootstrap seeding the six starters, with owner-dependent drafts paused
// (UAT.md:72).

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/modules/automation"
)

func TestAutomationCatalogAndCRUD(t *testing.T) {
	e := apptest.SetupAppWithOptions(t, compose.WithListsEnabled(true))
	e.BootstrapWorkspace(t)

	assertAutomationCatalogIsClosed(t, e)
	assertBootstrapSeededStarters(t, e)
	assertAutomationCreateValidatesParams(t, e)
	createdID := createPausedAutomation(t, e)

	// Enable under a stale If-Match is refused; the current version wins.
	stale := map[string]string{"If-Match": "99"}
	if status := e.Call(t, "PATCH", "/v1/automations/"+createdID, AnyMap{"status": "enabled"}, stale, nil); status != http.StatusConflict {
		t.Fatalf("stale If-Match → %d, want 409 version_skew", status)
	}
	var updated struct {
		Status  string `json:"status"`
		Version int    `json:"version"`
	}
	if status := e.Call(t, "PATCH", "/v1/automations/"+createdID, AnyMap{"status": "enabled"},
		map[string]string{"If-Match": "1"}, &updated); status != http.StatusOK {
		t.Fatalf("enable → %d", status)
	}
	if updated.Status != "enabled" || updated.Version != 2 {
		t.Fatalf("enable landed as %+v, want enabled v2", updated)
	}

	// Delete is a soft archive: 204, then the instance reads as absent.
	if status := e.Call(t, "DELETE", "/v1/automations/"+createdID, nil, nil, nil); status != http.StatusNoContent {
		t.Fatalf("delete → %d", status)
	}
	if status := e.Call(t, "GET", "/v1/automations/"+createdID, nil, nil, nil); status != http.StatusNotFound {
		t.Fatalf("get after delete → %d, want 404", status)
	}

	// Config is an audited fact end to end.
	var audit struct {
		Data []AnyMap `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/audit-log?entity_type=automation", nil, nil, &audit); status != http.StatusOK {
		t.Fatalf("audit read → %d", status)
	}
	if len(audit.Data) != 3 {
		t.Fatalf("automation config audited %d times, want 3 (create, enable, archive)", len(audit.Data))
	}
}

// The served catalog must include every registered recipe exactly once.
func assertAutomationCatalogIsClosed(t *testing.T, e *apptest.AppEnv) {
	t.Helper()
	// The catalog is the closed authorable library.
	var catalog struct {
		Data []struct {
			Key          string         `json:"key"`
			Tier         string         `json:"tier"`
			ParamsSchema map[string]any `json:"params_schema"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/automations/catalog", nil, nil, &catalog); status != http.StatusOK {
		t.Fatalf("catalog → %d", status)
	}
	expected := map[string]automation.CatalogEntry{}
	for _, entry := range automation.Catalog() {
		expected[entry.Key] = entry
	}
	if len(expected) == 0 || len(catalog.Data) != len(expected) {
		t.Fatalf("catalog carries %d types, registry has %d", len(catalog.Data), len(expected))
	}
	for _, entry := range catalog.Data {
		registered, ok := expected[entry.Key]
		if !ok {
			t.Fatalf("unexpected or repeated recipe %q", entry.Key)
		}
		if entry.Tier != registered.Tier {
			t.Fatalf("recipe %s tier = %q, want %q", entry.Key, entry.Tier, registered.Tier)
		}
		if entry.ParamsSchema == nil {
			t.Fatalf("starter %s ships no params_schema — the editor form has nothing to render", entry.Key)
		}
		delete(expected, entry.Key)
	}
}

// Starters that need configuration or draft mail wait for human review.
func assertBootstrapSeededStarters(t *testing.T, e *apptest.AppEnv) {
	t.Helper()
	var listed struct {
		Data []struct {
			ID      string `json:"id"`
			Key     string `json:"key"`
			Status  string `json:"status"`
			Version int    `json:"version"`
		} `json:"data"`
	}
	if status := e.Call(t, "GET", "/v1/automations", nil, nil, &listed); status != http.StatusOK {
		t.Fatalf("list → %d", status)
	}
	expected := map[string]automation.CatalogEntry{}
	for _, entry := range automation.Catalog() {
		if entry.Seeded {
			expected[entry.Key] = entry
		}
	}
	if len(expected) == 0 || len(listed.Data) != len(expected) {
		t.Fatalf("bootstrap seeded %d automations, want %d", len(listed.Data), len(expected))
	}
	for _, a := range listed.Data {
		entry, ok := expected[a.Key]
		if !ok {
			t.Fatalf("unexpected or repeated starter %q", a.Key)
		}
		want := "enabled"
		if entry.Validate(nil) != nil || entry.Action == string(automation.ActionTypeDraftEmail) {
			want = "paused"
		}
		if a.Status != want {
			t.Fatalf("seeded %s is %s, want %s", a.Key, a.Status, want)
		}
		delete(expected, a.Key)
	}
}

// assertAutomationCreateValidatesParams checks create refuses anything
// outside the catalog contract: unknown keys, mistyped params, and
// out-of-schema knobs (the anti-DSL guard) are all 422s.
func assertAutomationCreateValidatesParams(t *testing.T, e *apptest.AppEnv) {
	t.Helper()
	// An unknown catalog key and out-of-schema params are 422s.
	if status := e.Call(t, "POST", "/v1/automations", AnyMap{
		"key": "invented_type", "name": "Nope", "params": AnyMap{},
	}, nil, nil); status != 422 {
		t.Fatalf("unknown key → %d, want 422", status)
	}
	if status := e.Call(t, "POST", "/v1/automations", AnyMap{
		"key": "assign_lead_owner", "name": "Bad params", "params": AnyMap{"cap_per_owner": "soon"},
	}, nil, nil); status != 422 {
		t.Fatalf("mistyped param → %d, want 422", status)
	}
	if status := e.Call(t, "POST", "/v1/automations", AnyMap{
		"key": "assign_lead_owner", "name": "Rogue knob", "params": AnyMap{"rule_body": "if x then y"},
	}, nil, nil); status != 422 {
		t.Fatalf("out-of-schema param → %d, want 422 (the anti-DSL guard)", status)
	}
}

// createPausedAutomation creates a valid assign_lead_owner instance,
// asserts it lands paused and round-trips its config, and returns its id.
func createPausedAutomation(t *testing.T, e *apptest.AppEnv) string {
	t.Helper()
	// A valid create lands PAUSED and round-trips.
	var created struct {
		ID      string         `json:"id"`
		Status  string         `json:"status"`
		Params  map[string]any `json:"params"`
		Version int            `json:"version"`
	}
	if status := e.Call(t, "POST", "/v1/automations", AnyMap{
		"key": "assign_lead_owner", "name": "Slow-lane routing",
		"params": AnyMap{"owners": []string{apptest.SessionUserID(t, e)}, "cap_per_owner": 3},
	}, nil, &created); status != http.StatusCreated {
		t.Fatalf("create → %d", status)
	}
	if created.Status != "paused" {
		t.Fatalf("created status = %s, want paused (enabling is a second deliberate step)", created.Status)
	}
	var fetched struct {
		Name   string         `json:"name"`
		Params map[string]any `json:"params"`
	}
	if status := e.Call(t, "GET", "/v1/automations/"+created.ID, nil, nil, &fetched); status != http.StatusOK {
		t.Fatalf("get → %d", status)
	}
	if fetched.Name != "Slow-lane routing" || fetched.Params["cap_per_owner"] != float64(3) {
		t.Fatalf("round-trip lost the config: %+v", fetched)
	}
	return created.ID
}

// An automation an agent sets up starts paused, so nothing runs until it is
// enabled, and its tool says to ask before that. A read-only passport sets up none.
func TestAnAgentsAutomationStartsPaused(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	var minted struct {
		Token string `json:"token"`
	}
	if status := e.Call(t, "POST", "/v1/passports", AnyMap{
		"label": "automation prober", "scopes": []string{"read", "write"},
	}, nil, &minted); status != http.StatusCreated {
		t.Fatalf("mint → %d", status)
	}
	bearer := map[string]string{"Authorization": "Bearer " + minted.Token}

	var created struct {
		Status string `json:"status"`
	}
	if status := e.Call(t, "POST", "/v1/automations", AnyMap{
		"key": "assign_lead_owner", "name": "Agent-made", "params": AnyMap{},
	}, bearer, &created); status != http.StatusCreated || created.Status != "paused" {
		t.Fatalf("agent create automation → %d %+v, want a paused automation", status, created)
	}

	var readOnly struct {
		Token string `json:"token"`
	}
	if status := e.Call(t, "POST", "/v1/passports", AnyMap{
		"label": "automation reader", "scopes": []string{"read"},
	}, nil, &readOnly); status != http.StatusCreated {
		t.Fatalf("mint → %d", status)
	}
	if status := e.Call(t, "POST", "/v1/automations", AnyMap{
		"key": "assign_lead_owner", "name": "Reader-made", "params": AnyMap{},
	}, map[string]string{"Authorization": "Bearer " + readOnly.Token}, nil); status != http.StatusForbidden {
		t.Fatalf("read-only create automation → %d, want 403", status)
	}
}

// An automation always has a name, params and a status. A null on one is
// refused by name, never answered 200 with nothing changed.
func TestAnAutomationRefusesANullItCannotHold(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	createdID := createPausedAutomation(t, e)

	for _, field := range []string{"name", "params", "status"} {
		var problem companyProblem
		status := e.Call(t, "PATCH", "/v1/automations/"+createdID, AnyMap{field: nil},
			map[string]string{"If-Match": "1"}, &problem)
		if status != http.StatusUnprocessableEntity || len(problem.Details.Errors) != 1 ||
			problem.Details.Errors[0].Field != field || problem.Details.Errors[0].Code != "field_not_clearable" {
			t.Errorf("PATCH %s: null = %d %+v, want 422 field_not_clearable on %s", field, status, problem, field)
		}
	}
}
