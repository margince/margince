// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

import (
	"maps"
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
	var list struct {
		ClientCheck bool `json:"client_check"`
	}
	if status := e.Call(t, http.MethodGet, "/v1/installation/agent-clients", nil, nil, &list); status != http.StatusOK {
		t.Fatalf("GET /v1/installation/agent-clients = %d, want 200", status)
	}
	return list.ClientCheck
}

func setClientCheck(t *testing.T, e *apptest.AppEnv, on bool) {
	t.Helper()
	if status := e.Call(t, http.MethodPatch, "/v1/installation/settings", AnyMap{"mcp_client_check": on}, nil, nil); status != http.StatusOK {
		t.Fatalf("admin PATCH {mcp_client_check: %v} = %d, want 200", on, status)
	}
}

// The switch is read beside the other agent_client switches, on the admin's
// client list. The settings answer every role reads stays free of it.
func TestTheClientCheckSwitchIsOffByDefaultAndReadOnTheClientList(t *testing.T) {
	e := apptest.SetupApp(t)
	e.BootstrapWorkspace(t)

	if readClientCheck(t, e) {
		t.Fatal("an untouched installation reads client_check = true, want the default false")
	}
	setClientCheck(t, e, true)
	if !readClientCheck(t, e) {
		t.Fatal("after the admin turned it on, the client list still reads false")
	}

	demoteToRep(t, e)
	if status := e.Call(t, http.MethodGet, "/v1/installation/settings", nil, nil, nil); status != http.StatusOK {
		t.Errorf("rep GET /v1/installation/settings = %d, want 200", status)
	}
	if status := e.Call(t, http.MethodGet, "/v1/installation/agent-clients", nil, nil, nil); status != http.StatusForbidden {
		t.Errorf("rep GET /v1/installation/agent-clients = %d, want 403", status)
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

// modernCheck calls check_client in the 2026-07-28 framing from a client that
// declares form elicitation, carrying extra params members on a retry.
func modernCheck(t *testing.T, e *apptest.AppEnv, bearer map[string]string, extra AnyMap) map[string]any {
	t.Helper()
	params := AnyMap{
		"name": clientCheckTool, "arguments": AnyMap{},
		"_meta": AnyMap{
			"io.modelcontextprotocol/protocolVersion":    "2026-07-28",
			"io.modelcontextprotocol/clientCapabilities": AnyMap{"elicitation": AnyMap{}},
			"io.modelcontextprotocol/clientInfo":         AnyMap{"name": "probe", "version": "1.0"},
		},
	}
	maps.Copy(params, extra)
	headers := map[string]string{"MCP-Protocol-Version": "2026-07-28", "Mcp-Method": "tools/call", "Mcp-Name": clientCheckTool}
	maps.Copy(headers, bearer)
	var answer struct {
		Result map[string]any `json:"result"`
	}
	body := AnyMap{"jsonrpc": "2.0", "id": 1, "method": "tools/call", "params": params}
	if status := e.Call(t, http.MethodPost, "/mcp", body, headers, &answer); status != http.StatusOK {
		t.Fatalf("modern tools/call %s = %d, want 200", clientCheckTool, status)
	}
	return answer.Result
}

// The tool exists for one caller only: a passport granted by an admin, while
// the switch is on. The gate is the one compose wires, read over /mcp, and the
// form goes out through the signed in-chat question every tool shares.
func TestTheClientCheckToolExistsOnlyForAnAdminWhileTheSwitchIsOn(t *testing.T) {
	e := apptest.SetupAppWithOriginOptions(t, func(origin string) []compose.Option {
		return []compose.Option{
			compose.WithMCPConnector(), compose.WithMCPResource(origin + "/mcp"),
			compose.WithMCPInputStateKey("client-check-integration-state-key-0123"),
		}
	})
	e.BootstrapWorkspace(t)
	client := apptest.NewMCPClient(e, apptest.MCPBearerToken(t, e, "client check", "read"))

	withheldLike(t, client, "switch off, admin")

	setClientCheck(t, e, true)
	if !slices.Contains(client.ListTools(t), clientCheckTool) {
		t.Fatalf("switch on, admin: tools/list leaves out %s", clientCheckTool)
	}
	asked := modernCheck(t, e, client.Bearer(), nil)
	state, signed := asked["requestState"].(string)
	if asked["resultType"] != "input_required" || !signed {
		t.Fatalf("switch on, admin, form client: answered %v, want input_required with a requestState", asked)
	}
	answer := AnyMap{"requestState": state, "inputResponses": AnyMap{
		"margince_client_check": AnyMap{"action": "accept", "content": AnyMap{"colour": "blue"}},
	}}
	got := modernCheck(t, e, client.Bearer(), answer)
	structured, _ := got["structuredContent"].(map[string]any)
	if data, _ := structured["data"].(map[string]any); data["supported"] != true || data["action"] != "accept" {
		t.Errorf("an accepted form was not measured as supported: %v", got)
	}

	demoteToRep(t, e)
	withheldLike(t, client, "switch on, rep")
}
