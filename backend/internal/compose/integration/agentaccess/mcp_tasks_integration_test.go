// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package agentaccess

// The io.modelcontextprotocol/tasks extension, end to end on the real origin
// against a real database — what is left of it.
//
// NOTHING REACHABLE THROUGH THE TOOL DOOR STAGES ANY MORE, so this suite can no
// longer mint a handle to prove anything about. #2426 moved 32 verbs to
// auto_execute under ADR-0055, on the argument that a passport already carries
// the granting human's own seat and grants. The three routes still declared
// confirm-first cannot be called as tools: create_record and update_record
// enumerate seven record types and custom_field and webhook_subscription are
// not among them, and enrich implements no RecordTypeOf, so it takes no floor
// (recordtyped.go is the set a floor can reach).
//
// The nine tests that lived here asserted the handle's durability across
// requests, exactly-once release, the recorded-result replay and the
// passport-binding. They were deleted rather than rewritten because there is no
// verb to rewrite them onto — a test that cannot be entered proves nothing, and
// one kept alive on a fixture the product cannot produce proves less than
// nothing. The unit suite still covers the state machine over fakes.
//
// What remains is the claim that survives: the extension is advertised, and its
// methods refuse a client that did not declare it.
//
// The gap this leaves is deliberate and recorded — the surface advertises a
// capability no tool call can enter (#2432). Restoring one is a product
// decision about what the agent surface promises, not a test edit.

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/margince/margince/backend/internal/compose/integration/apptest"
)

// The per-request metadata a TASK-CAPABLE modern client sends, spelled as the
// specification writes it — this suite is a client, so it must not read the
// server's own constants for what to put on the wire.
const tasksMeta = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28",` +
	`"io.modelcontextprotocol/clientCapabilities":{"extensions":{"io.modelcontextprotocol/tasks":{}}}}`

// The extension is advertised where a client can act on it, and the three
// methods refuse a request that did not declare it.
func TestTheExtensionIsAdvertisedAndItsMethodsRequireIt(t *testing.T) {
	e := setupConnector(t)
	bearer := apptest.PassportBearer(t, e.AppEnv, "task client", "read")

	discovered := rpcResult(t, mcpRaw(e.AppEnv, t, http.MethodPost, "/mcp",
		fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{%s}}`, tasksMeta),
		modernHeaders(bearer, "server/discover", "")).Body)
	capabilities, _ := discovered["capabilities"].(map[string]any)
	extensions, ok := capabilities["extensions"].(map[string]any)
	if !ok {
		t.Fatalf("server/discover advertised no extensions: %v", capabilities)
	}
	if _, named := extensions["io.modelcontextprotocol/tasks"]; !named {
		t.Fatalf("extensions = %v, want io.modelcontextprotocol/tasks", extensions)
	}

	// And a request that did not declare it is asking for a method that, for
	// that caller, does not exist.
	undeclared := mcpRaw(e.AppEnv, t, http.MethodPost, "/mcp",
		fmt.Sprintf(`{"jsonrpc":"2.0","id":2,"method":"tasks/get","params":{%s,"taskId":"019fe000-0000-7000-8000-00000000dead"}}`, modernMeta),
		modernHeaders(bearer, "tasks/get", "019fe000-0000-7000-8000-00000000dead"))
	code, message := rpcErrorOf(t, undeclared.Body)
	if code != -32021 {
		t.Fatalf("tasks/get without the capability → %d %q, want -32021 "+
			"(the core specification's MissingRequiredClientCapability)", code, message)
	}
}

// rpcErrorOf decodes the ERROR half of a JSON-RPC response, which rpcResult
// deliberately refuses to do — several assertions here are about the refusal.
func rpcErrorOf(t *testing.T, body string) (int, string) {
	t.Helper()
	var envelope struct {
		Error *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(body), &envelope); err != nil {
		t.Fatalf("JSON-RPC response does not decode: %v (%s)", err, body)
	}
	if envelope.Error == nil {
		t.Fatalf("expected a JSON-RPC error, got: %s", body)
	}
	return envelope.Error.Code, envelope.Error.Message
}
