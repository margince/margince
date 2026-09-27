// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The role editor's lifecycle over HTTP, as the bootstrap admin: create by
// copy, rename under If-Match, archive with its refusals, and a custom role
// handed to an invited member.

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

type roleWire struct {
	Key        string  `json:"key"`
	Name       string  `json:"name"`
	IsSystem   bool    `json:"is_system"`
	Version    int64   `json:"version"`
	RowScope   string  `json:"row_scope"`
	ArchivedAt *string `json:"archived_at"`
}

type roleDirectoryWire struct {
	Roles []roleWire `json:"roles"`
}

func TestTheRoleLifecycleOverHTTP(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)

	var created roleWire
	if status := e.Call(t, "POST", "/v1/roles", map[string]any{"copy_from": "rep", "name": "Field sales"}, nil, &created); status != http.StatusCreated {
		t.Fatalf("create -> %d, want 201", status)
	}
	if created.Key != "custom_field_sales" || created.IsSystem || created.RowScope != "own" {
		t.Fatalf("created = %+v, want custom_field_sales, not system, rep's own scope", created)
	}
	var taken refusalWire
	if status := e.Call(t, "POST", "/v1/roles", map[string]any{"copy_from": "rep", "name": "FIELD SALES"}, nil, &taken); status != http.StatusConflict {
		t.Fatalf("a second role with the name -> %d, want 409", status)
	}
	assertActionableRefusal(t, "a taken role name", taken, "role_name_taken")

	base := "/v1/roles/" + created.Key
	stale := map[string]string{"If-Match": strconv.FormatInt(created.Version, 10)}
	var renamed roleWire
	if status := e.Call(t, "PATCH", base, map[string]any{"name": "Field sales DACH", "row_scope": "team"}, stale, &renamed); status != http.StatusOK {
		t.Fatalf("rename -> %d, want 200", status)
	}
	if renamed.Name != "Field sales DACH" || renamed.RowScope != "team" {
		t.Errorf("renamed = %+v, want the new name at team scope", renamed)
	}
	var skew refusalWire
	if status := e.Call(t, "PATCH", base, map[string]any{"name": "Again"}, stale, &skew); status != http.StatusConflict {
		t.Fatalf("a write from the stale version -> %d, want 409", status)
	}

	var system refusalWire
	if status := e.Call(t, "POST", "/v1/roles/rep/archive", nil, nil, &system); status != http.StatusConflict {
		t.Fatalf("archiving rep -> %d, want 409", status)
	}
	assertActionableRefusal(t, "archiving a system role", system, "system_role")

	var invited userWire
	if status := e.Call(t, "POST", "/v1/users", map[string]any{
		"email": "field@acme.test", "display_name": "Field Rep", "role": created.Key,
	}, nil, &invited); status != http.StatusCreated {
		t.Fatalf("inviting with a custom role -> %d, want 201", status)
	}
	assertRoles(t, "invite with a custom role", invited, created.Key)
	var inUse refusalWire
	if status := e.Call(t, "POST", base+"/archive", nil, nil, &inUse); status != http.StatusConflict {
		t.Fatalf("archiving a held role -> %d, want 409", status)
	}
	assertActionableRefusal(t, "archiving a held role", inUse, "role_in_use")

	if status := e.Call(t, "PATCH", "/v1/users/"+invited.ID+"/role", map[string]any{"role": "rep"}, nil, nil); status != http.StatusOK {
		t.Fatalf("re-roling the holder -> %d, want 200", status)
	}
	var archived roleWire
	if status := e.Call(t, "POST", base+"/archive", nil, nil, &archived); status != http.StatusOK || archived.ArchivedAt == nil {
		t.Fatalf("archiving a free role -> %d %+v, want 200 with archived_at", status, archived)
	}
	var live, every roleDirectoryWire
	e.Call(t, "GET", "/v1/roles", nil, nil, &live)
	e.Call(t, "GET", "/v1/roles?include_archived=true", nil, nil, &every)
	if hasRole(live, created.Key) || !hasRole(every, created.Key) {
		t.Errorf("the archived role is listed live=%v, with archived=%v; want false, true",
			hasRole(live, created.Key), hasRole(every, created.Key))
	}
	var gone refusalWire
	if status := e.Call(t, "PATCH", "/v1/users/"+invited.ID+"/role", map[string]any{"role": created.Key}, nil, &gone); status != http.StatusNotFound {
		t.Fatalf("assigning an archived role -> %d, want 404", status)
	}
	assertActionableRefusal(t, "assigning an archived role", gone, "unknown_role")
}

func hasRole(directory roleDirectoryWire, key string) bool {
	for _, role := range directory.Roles {
		if role.Key == key {
			return true
		}
	}
	return false
}
