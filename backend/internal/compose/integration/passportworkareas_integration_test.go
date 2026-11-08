// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// A passport limited to some areas of work is held to them on both doors and in
// the tool list. A change to its areas holds from its next call. Each step runs
// through the real stack: the passport route, /mcp and /v1.

import (
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The admin-settings read the test is refused, and the sales read it keeps.
// Both are read_workspace topics, so one router shows the narrowing.
const (
	adminTopic = "roles"
	adminRoute = "/v1/roles"
	salesTopic = "saved_views"
	salesRoute = "/v1/views"
)

type mintedPassport struct {
	ID    string `json:"passport_id"`
	Token string `json:"token"`
}

func areasApp(t *testing.T) *apptest.AppEnv {
	t.Helper()
	e := apptest.SetupAppWithOriginOptions(t, func(origin string) []compose.Option {
		return []compose.Option{compose.WithMCPConnector(), compose.WithMCPResource(origin + "/mcp")}
	})
	e.BootstrapWorkspace(t)
	return e
}

func mintWithAreas(t *testing.T, e *apptest.AppEnv, areas AnyMap) mintedPassport {
	t.Helper()
	body := AnyMap{"label": "areas probe", "scopes": []string{"read", "write"}}
	if areas != nil {
		body["areas"] = areas
	}
	var minted mintedPassport
	if status := e.Call(t, http.MethodPost, "/v1/passports", body, nil, &minted); status != http.StatusCreated {
		t.Fatalf("issue passport → %d", status)
	}
	return minted
}

// topicsOffered is the read_workspace topic enum this credential is listed.
func topicsOffered(t *testing.T, client *apptest.MCPClient) []string {
	t.Helper()
	for _, tool := range client.ListedTools(t) {
		if tool.Name != "read_workspace" {
			continue
		}
		var schema struct {
			Properties struct {
				Topic struct {
					Enum []string `json:"enum"`
				} `json:"topic"`
			} `json:"properties"`
		}
		if err := json.Unmarshal(tool.InputSchema, &schema); err != nil {
			t.Fatalf("read_workspace's schema does not read: %v", err)
		}
		return schema.Properties.Topic.Enum
	}
	return nil
}

func TestAPassportIsHeldToTheAreasItsUserChose(t *testing.T) {
	e := areasApp(t)
	minted := mintWithAreas(t, e, AnyMap{"mode": "custom", "areas": []string{"business"}})
	bearer := map[string]string{"Authorization": "Bearer " + minted.Token}
	client := apptest.NewMCPClient(e, minted.Token)

	var problem struct {
		Code   string `json:"code"`
		Detail string `json:"detail"`
	}
	if status := e.Call(t, http.MethodGet, adminRoute, nil, bearer, &problem); status != http.StatusForbidden ||
		problem.Code != "scope_exceeds_grantor" || !strings.Contains(problem.Detail, "“Admin settings”") {
		t.Fatalf("REST %s with admin settings off → %d %+v, want 403 naming the area", adminRoute, status, problem)
	}
	if status := e.Call(t, http.MethodGet, salesRoute, nil, bearer, nil); status != http.StatusOK {
		t.Fatalf("REST %s with sales work on → %d, want 200", salesRoute, status)
	}
	// Who the caller is sits in no area, on REST as it does over MCP.
	if status := e.Call(t, http.MethodGet, "/v1/whoami", nil, bearer, nil); status != http.StatusOK {
		t.Fatalf("REST /v1/whoami with my account off → %d, want 200", status)
	}

	topics := topicsOffered(t, client)
	if slices.Contains(topics, adminTopic) || !slices.Contains(topics, salesTopic) {
		t.Fatalf("read_workspace lists topics %v; want %s and not %s", topics, salesTopic, adminTopic)
	}
	refusal := client.CallRefused(t, "read_workspace", AnyMap{"topic": adminTopic})
	if !strings.Contains(refusal, "“Admin settings”") || !strings.Contains(refusal, "Settings → AI access") {
		t.Fatalf("MCP read_workspace %s refused with %q; want the area and where to turn it on", adminTopic, refusal)
	}
	client.CallOK(t, "read_workspace", AnyMap{"topic": salesTopic})

	// Turned back on by its user, the area holds from the next call on the same
	// credential: no reconnect, no new token.
	var stored struct {
		Mode  string   `json:"mode"`
		Areas []string `json:"areas"`
	}
	if status := e.Call(t, http.MethodPut, "/v1/passports/"+minted.ID+"/areas",
		AnyMap{"mode": "custom", "areas": []string{"business", "admin_settings"}}, nil, &stored); status != http.StatusOK ||
		stored.Mode != "custom" || !slices.Equal(stored.Areas, []string{"business", "admin_settings"}) {
		t.Fatalf("turning admin settings on → %d %+v", status, stored)
	}
	if status := e.Call(t, http.MethodGet, adminRoute, nil, bearer, nil); status != http.StatusOK {
		t.Errorf("REST %s once admin settings is on → %d, want 200", adminRoute, status)
	}
	client.CallOK(t, "read_workspace", AnyMap{"topic": adminTopic})
	if topics := topicsOffered(t, client); !slices.Contains(topics, adminTopic) {
		t.Errorf("read_workspace lists %v once admin settings is on; want %s back", topics, adminTopic)
	}
}

// Every passport starts at all areas, stored as NULL so an area added later
// is reached, and the list says so.
func TestAPassportStartsAtAllAreas(t *testing.T) {
	e := areasApp(t)
	minted := mintWithAreas(t, e, nil)

	var column []string
	if err := e.Owner.QueryRow(t.Context(), `SELECT work_areas FROM passport WHERE id = $1`, minted.ID).Scan(&column); err != nil {
		t.Fatalf("reading the stored areas: %v", err)
	}
	if column != nil {
		t.Fatalf("a passport minted without areas stores %v, want NULL", column)
	}
	var listed struct {
		Data []struct {
			ID    string `json:"id"`
			Areas struct {
				Mode string `json:"mode"`
			} `json:"areas"`
		} `json:"data"`
		Reachable []string `json:"reachable_areas"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/passports", nil, nil, &listed); status != http.StatusOK {
		t.Fatalf("list passports → %d", status)
	}
	if len(listed.Data) != 1 || listed.Data[0].Areas.Mode != "all" {
		t.Fatalf("the list shows %+v, want the one passport at all areas", listed.Data)
	}
	if !slices.Contains(listed.Reachable, "admin_settings") || !slices.Contains(listed.Reachable, "business") {
		t.Errorf("an admin reaches %v; want sales work and admin settings among them", listed.Reachable)
	}
	bearer := map[string]string{"Authorization": "Bearer " + minted.Token}
	if status := e.Call(t, http.MethodGet, adminRoute, nil, bearer, nil); status != http.StatusOK {
		t.Errorf("REST %s on a passport at all areas → %d, want 200", adminRoute, status)
	}
}

// Both doors refuse a choice the screen never offers, and another user's
// passport reads as absent.
func TestAnAreaChoiceNoScreenOffersIsRefused(t *testing.T) {
	e := areasApp(t)
	minted := mintWithAreas(t, e, nil)
	for name, body := range map[string]AnyMap{
		"no area":          {"mode": "custom", "areas": []string{}},
		"an operator area": {"mode": "custom", "areas": []string{"operator"}},
		"an unknown mode":  {"mode": "some", "areas": []string{}},
		"all with a list":  {"mode": "all", "areas": []string{"business"}},
	} {
		if status := e.Call(t, http.MethodPut, "/v1/passports/"+minted.ID+"/areas", body, nil, nil); status != http.StatusUnprocessableEntity {
			t.Errorf("%s → %d, want 422", name, status)
		}
	}
	if status := e.Call(t, http.MethodPut, "/v1/passports/"+ids.NewV7().String()+"/areas",
		AnyMap{"mode": "all", "areas": []string{}}, nil, nil); status != http.StatusNotFound {
		t.Errorf("a passport that is not the caller's → %d, want 404", status)
	}
	bearer := map[string]string{"Authorization": "Bearer " + minted.Token}
	if status := e.Call(t, http.MethodPut, "/v1/passports/"+minted.ID+"/areas",
		AnyMap{"mode": "all", "areas": []string{}}, bearer, nil); status != http.StatusForbidden {
		t.Errorf("a passport widening its own areas → %d, want 403", status)
	}
}

// A rep reaches every area but admin settings. A choice of an area the role
// does not reach is refused on each door that takes one.
func TestARepChoosesOnlyAmongTheAreasTheirRoleReaches(t *testing.T) {
	e := areasApp(t)
	demoteToRep(t, e)

	var listed struct {
		Reachable []string `json:"reachable_areas"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/passports", nil, nil, &listed); status != http.StatusOK {
		t.Fatalf("list passports → %d", status)
	}
	want := []string{"business", "compliance", "capture", "workspace_settings", "own_account"}
	if !slices.Equal(listed.Reachable, want) {
		t.Fatalf("a rep reaches %v, want %v", listed.Reachable, want)
	}

	adminOnly := AnyMap{"mode": "custom", "areas": []string{"admin_settings"}}
	status := e.Call(t, http.MethodPost, "/v1/passports", AnyMap{"label": "rep", "scopes": []string{"read"}, "areas": adminOnly}, nil, nil)
	if status != http.StatusUnprocessableEntity {
		t.Errorf("a rep minting a passport limited to admin settings → %d, want 422", status)
	}
	minted := mintWithAreas(t, e, nil)
	if status := e.Call(t, http.MethodPut, "/v1/passports/"+minted.ID+"/areas", adminOnly, nil, nil); status != http.StatusUnprocessableEntity {
		t.Errorf("a rep limiting a passport to admin settings → %d, want 422", status)
	}
	if status := e.Call(t, http.MethodPut, "/v1/passports/"+minted.ID+"/areas",
		AnyMap{"mode": "custom", "areas": []string{"business"}}, nil, nil); status != http.StatusOK {
		t.Errorf("a rep limiting a passport to sales work → %d, want 200", status)
	}
}
