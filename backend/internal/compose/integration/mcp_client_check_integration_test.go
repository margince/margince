// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"net/http"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

const clientCheckTool = "check_client"

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

func setClientCheck(t *testing.T, e *apptest.AppEnv, on bool) {
	t.Helper()
	if status := e.Call(t, http.MethodPatch, "/v1/installation/settings", AnyMap{"mcp_client_check": on}, nil, nil); status != http.StatusOK {
		t.Fatalf("admin PATCH {mcp_client_check: %v} = %d, want 200", on, status)
	}
}

// Every role reads the settings answer, and the switch lives under agent_client,
// which only an admin reads. So the answer must carry the switch to a rep too.
func TestTheClientCheckSwitchIsOffByDefaultAndReadByEveryRole(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	if readClientCheck(t, e) {
		t.Fatal("an untouched installation reads mcp_client_check = true, want the default false")
	}
	setClientCheck(t, e, true)
	if !readClientCheck(t, e) {
		t.Fatal("after the admin turned it on, the settings answer still reads false")
	}

	demoteToRep(t, e)
	if !readClientCheck(t, e) {
		t.Error("a rep reads the switch as off, or cannot read the settings answer at all")
	}
}

// The switch is held by agent_client update, not by the right to change the
// installation's other settings. A rep granted installation_settings update
// moves a messenger switch and is still refused this one.
func TestOnlyAgentClientUpdateMovesTheClientCheckSwitch(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)
	demoteToRep(t, e)
	if _, err := e.Owner.Exec(t.Context(),
		`UPDATE role SET permissions = jsonb_set(permissions, '{objects,installation_settings}',
		   '{"read":true,"create":false,"update":true,"delete":false}'::jsonb, true),
		   version = version + 1
		 WHERE key = $1`, roleRep); err != nil {
		t.Fatalf("granting the rep installation_settings update: %v", err)
	}

	if status := e.Call(t, http.MethodPatch, "/v1/installation/settings", AnyMap{"call_zalo": true}, nil, nil); status != http.StatusOK {
		t.Fatalf("rep PATCH {call_zalo: true} = %d, want 200: the grant did not reach the session", status)
	}
	if status := e.Call(t, http.MethodPatch, "/v1/installation/settings", AnyMap{"mcp_client_check": true}, nil, nil); status != http.StatusForbidden {
		t.Errorf("rep PATCH {mcp_client_check: true} = %d, want 403", status)
	}
}

// withheldLike asserts check_client is not listed, and that a call to it gets
// the words an unregistered name gets. The refusal must not say it exists.
func withheldLike(t *testing.T, client *apptest.MCPClient, situation string) {
	t.Helper()
	if slices.Contains(client.ListTools(t), clientCheckTool) {
		t.Errorf("%s: tools/list offers %s", situation, clientCheckTool)
	}
	const nonexistent = "no_such_tool"
	absent := strings.ReplaceAll(client.CallRefused(t, nonexistent, nil), nonexistent, "<name>")
	withheld := strings.ReplaceAll(client.CallRefused(t, clientCheckTool, nil), clientCheckTool, "<name>")
	if withheld != absent {
		t.Errorf("%s: a withheld call answers unlike an unknown name:\n%s\n%s", situation, withheld, absent)
	}
}

// The tool exists for one caller only: a passport granted by an admin, while
// the switch is on. The gate is the one compose wires, read over /mcp.
func TestTheClientCheckToolExistsOnlyForAnAdminWhileTheSwitchIsOn(t *testing.T) {
	e := apptest.SetupAppWithOriginOptions(t, func(origin string) []compose.Option {
		return []compose.Option{compose.WithMCPConnector(), compose.WithMCPResource(origin + "/mcp")}
	})
	e.BootstrapWorkspace(t)
	client := apptest.NewMCPClient(e, apptest.MCPBearerToken(t, e, "client check", "read"))

	withheldLike(t, client, "switch off, admin")

	setClientCheck(t, e, true)
	if !slices.Contains(client.ListTools(t), clientCheckTool) {
		t.Fatalf("switch on, admin: tools/list leaves out %s", clientCheckTool)
	}
	client.CallOK(t, clientCheckTool, nil)

	demoteToRep(t, e)
	withheldLike(t, client, "switch on, rep")
}
