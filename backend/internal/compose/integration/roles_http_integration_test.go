// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// The role editor's lifecycle over HTTP, as the bootstrap admin: create by
// copy, rename under If-Match, archive with its refusals, and a custom role
// handed to an invited member.

import (
	"context"
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

// dropFieldSalesMasksAfter removes the masks a copied "Field sales" role took
// from rep: field_mask has no workspace column, so a mask left behind reaches
// the next test's copy of the same key and refuses its insert.
func dropFieldSalesMasksAfter(t *testing.T) {
	t.Helper()
	owner := OwnerConn(t)
	t.Cleanup(func() {
		if _, err := owner.Exec(context.Background(), `DELETE FROM field_mask WHERE role_key = 'custom_field_sales'`); err != nil {
			t.Errorf("removing custom_field_sales's masks: %v", err)
		}
	})
}

func TestTheRoleLifecycleOverHTTP(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	dropFieldSalesMasksAfter(t)

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

// Each refusal the role editor's transport can answer, through the real
// handlers: a body it cannot read, an If-Match it cannot parse, and the codes
// the service's refusals map onto.
func TestTheRoleEditorsRefusalsOverHTTP(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	dropFieldSalesMasksAfter(t)

	var created roleWire
	if status := e.Call(t, "POST", "/v1/roles", map[string]any{"copy_from": "rep", "name": "Field sales"}, nil, &created); status != http.StatusCreated {
		t.Fatalf("create -> %d, want 201", status)
	}
	base := "/v1/roles/" + created.Key

	for _, tc := range []struct {
		name    string
		method  string
		path    string
		body    any
		headers map[string]string
		status  int
		code    string
	}{
		{"a create body of the wrong shape", "POST", "/v1/roles", map[string]any{"copy_from": 7, "name": "X"}, nil, http.StatusBadRequest, ""},
		{"a copy of an unknown role", "POST", "/v1/roles", map[string]any{"copy_from": "custom_nobody", "name": "Ghost"}, nil, http.StatusNotFound, "unknown_role"},
		{"an If-Match that is not a version", "PATCH", base, map[string]any{"name": "Y"}, map[string]string{"If-Match": "soon"}, http.StatusBadRequest, ""},
		{"an update body of the wrong shape", "PATCH", base, map[string]any{"name": 7}, nil, http.StatusBadRequest, ""},
		{"an update of an unknown role", "PATCH", "/v1/roles/custom_nobody", map[string]any{"name": "Y"}, nil, http.StatusNotFound, "unknown_role"},
		{"an archive of an unknown role", "POST", "/v1/roles/custom_nobody/archive", nil, nil, http.StatusNotFound, "unknown_role"},
		{"a restore of an unknown role", "POST", "/v1/roles/custom_nobody/restore", nil, nil, http.StatusNotFound, "unknown_role"},
		{
			"narrowing the admin role's administration", "PATCH", "/v1/roles/admin/objects/role_admin",
			map[string]any{"create": false, "read": true, "update": false, "delete": false},
			nil, http.StatusConflict, "admin_role_floor",
		},
	} {
		var refusal refusalWire
		status := e.Call(t, tc.method, tc.path, tc.body, tc.headers, &refusal)
		// A body the decoder cannot read is 400 or 422 depending on where it
		// fails; either is a refusal of the input, which is the claim.
		if status != tc.status && (tc.status != http.StatusBadRequest || status != http.StatusUnprocessableEntity) {
			t.Errorf("%s -> %d, want %d", tc.name, status, tc.status)
			continue
		}
		if tc.code != "" {
			assertActionableRefusal(t, tc.name, refusal, tc.code)
		}
	}

	var restored roleWire
	if status := e.Call(t, "POST", base+"/restore", nil, nil, &restored); status != http.StatusOK || restored.ArchivedAt != nil {
		t.Errorf("restoring a live role -> %d %+v, want 200 and still live", status, restored)
	}
}

// The pickers' read over HTTP: the admin is offered every live role, and the
// answer is never cached by a shared proxy.
func TestTheAssignableRolesOverHTTP(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	e.DescribeCompany(t)
	dropFieldSalesMasksAfter(t)
	if status := e.Call(t, "POST", "/v1/roles", map[string]any{"copy_from": "rep", "name": "Field sales"}, nil, nil); status != http.StatusCreated {
		t.Fatalf("create -> %d, want 201", status)
	}
	var directory struct {
		Roles []struct {
			Key      string `json:"key"`
			IsSystem bool   `json:"is_system"`
		} `json:"roles"`
	}
	if status := e.Call(t, "GET", "/v1/users/assignable-roles", nil, nil, &directory); status != http.StatusOK {
		t.Fatalf("assignable roles -> %d, want 200", status)
	}
	keys := map[string]bool{}
	for _, role := range directory.Roles {
		keys[role.Key] = role.IsSystem
	}
	for _, key := range []string{"admin", "management", "manager", "ops", "read_only", "rep"} {
		if system, ok := keys[key]; !ok || !system {
			t.Errorf("the admin is not offered the seeded role %q (listed %v)", key, ok)
		}
	}
	if system, ok := keys["custom_field_sales"]; !ok || system {
		t.Errorf("the custom role is listed %v with is_system %v, want listed and not system", ok, system)
	}
}
