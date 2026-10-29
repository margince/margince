// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

func readClientCheck(t *testing.T, e *apptest.AppEnv) bool {
	t.Helper()
	var settings struct {
		McpClientCheck bool `json:"mcp_client_check"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/installation/settings", nil, nil, &settings); status != http.StatusOK {
		t.Fatalf("GET /v1/installation/settings = %d, want 200", status)
	}
	return settings.McpClientCheck
}

// Every role reads the settings answer, and the switch lives under agent_client,
// which only an admin reads. So the answer must carry the switch to a rep too,
// and only an admin may move it.
func TestTheClientCheckSwitchIsOffByDefaultReadByEveryRoleAndMovedOnlyByAnAdmin(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	if readClientCheck(t, e) {
		t.Fatal("an untouched installation reads mcp_client_check = true, want the default false")
	}
	if status := e.Call(t, http.MethodPatch, "/v1/installation/settings", AnyMap{"mcp_client_check": true}, nil, nil); status != http.StatusOK {
		t.Fatalf("admin PATCH {mcp_client_check: true} = %d, want 200", status)
	}
	if !readClientCheck(t, e) {
		t.Fatal("after the admin turned it on, the settings answer still reads false")
	}

	demoteToRep(t, e)
	if !readClientCheck(t, e) {
		t.Error("a rep reads the switch as off, or cannot read the settings answer at all")
	}
	if status := e.Call(t, http.MethodPatch, "/v1/installation/settings", AnyMap{"mcp_client_check": false}, nil, nil); status != http.StatusForbidden {
		t.Errorf("rep PATCH {mcp_client_check: false} = %d, want 403", status)
	}
}
