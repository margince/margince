// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package humanonlyext

// x-agent-access: human-only, end to end over a real composed app and real
// Postgres: the mechanism openchannel's 7 operations now use to stay
// REST/UI-reachable but never MCP/agent-reachable. DESIGN.md's "Data flow"
// table is the exact set of cases this proves:
//
//   - Human, REST                            -> 200
//   - Agent, REST                            -> 403 permission_denied
//   - Agent, MCP tools/list                  -> tool absent
//
// Against a SYNTHETIC human-only extension (built with extension.Extension/
// extension.Verb, the same construction the compose package's own white-box
// fixtures use — extensiontools_test.go's unitVerb, extjobs_integration_test.go)
// rather than the real openchannel unit: the arch-lint DAG forbids compose
// (which this suite's own package lives under, internal/compose/integration/…)
// from importing the generated "composition" module — that import is reserved
// for cmd/api and cmd/worker, the two real boot entry points. A synthetic
// verb carrying the identical shape (HumanOnly, an RBAC object, a mutating
// method) proves the exact same mechanism openchannel's real contract fragment
// declares; extverbs_test.go and verb_test.go already prove the parsing and
// validation side of openchannel's actual YAML.
//
// OWN PACKAGE, deliberately, not a file under internal/compose/integration
// alongside the several hundred other suites there. compose.RegisterExtensions
// is a package-level, process-wide reconciliation (extensions.go's own doc) —
// it stays applied for the rest of the test BINARY, not just this test — and
// registering even a synthetic unit there would make it visible to that
// package's own whole-surface census
// (toolschema_conformance_integration_test.go), which would then fail on a
// tool nothing in its scenario list calls. A dedicated test binary contains
// the registration to exactly the suite that needs it — the same reason
// internal/compose/integration/webhooks, /capture, /collections are their own
// packages rather than files in the parent one.

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/backend/pkg/extension"
)

// humanOnlyVerb is the synthetic operation under test: a mutating,
// human-only extension operation shaped exactly like openchannel_open —
// HumanOnly, an RBAC object, no Tier/RequestedScope.
var humanOnlyVerb = extension.Verb{
	Unit:        "demoext",
	Contract:    "crm.yaml",
	OperationID: "demoextOpen",
	Route:       "/ext/demoext/open",
	Method:      http.MethodPut,
	Tool:        "demoext_open",
	Title:       "Open a demo endpoint",
	Description: "A synthetic human-only operation, standing in for openchannel_open's shape.",
	Version:     "1.0.0",
	HumanOnly:   true,
	RbacObject:  "ext_demoext_endpoint",
	RbacAction:  extension.RbacCreate,
}

// humanOnlyHandle answers a fixed body — the point of this suite is the
// enforcement mechanism around the call, not what a handler does with it.
func humanOnlyHandle(context.Context, extension.Runtime, json.RawMessage) (json.RawMessage, error) {
	return json.RawMessage(`{"opened":true}`), nil
}

// registerDemoExtOnce guards compose.RegisterExtensions: jurisdiction.Register
// beneath it refuses a second registration of the same pack outright, so every
// test in this binary that needs the composed set shares one call rather than
// each racing to be first.
var registerDemoExtOnce sync.Once

// bootWithDemoExt registers the synthetic human-only extension into the core
// registries before booting the app harness — this must run before
// compose.New assembles anything that reads it, and apptest.SetupApp's
// harness never calls it itself (only cmd/api and cmd/worker do, at real
// boot, with the real composed set).
func bootWithDemoExt(t *testing.T, opts ...compose.Option) *apptest.AppEnv {
	t.Helper()
	var registerErr error
	registerDemoExtOnce.Do(func() {
		registerErr = compose.RegisterExtensions([]extension.Extension{{
			Name:        "demoext",
			Version:     "1.0.0",
			Description: "A synthetic unit, composed only for this suite's own test binary.",
			Tools:       []extension.Tool{{Name: "demoext_open", Handle: humanOnlyHandle}},
		}}, []extension.Verb{humanOnlyVerb}, nil)
	})
	if registerErr != nil {
		t.Fatalf("registering the synthetic extension set: %v", registerErr)
	}
	e := apptest.SetupAppWithOptions(t, opts...)
	// The per-call Runtime a served extension tool's Handle receives
	// (pool + vault) — cmd/api and cmd/worker both bind it at boot
	// (keyvault.go, boot.go); apptest's harness has no equivalent call, so a
	// suite whose subject IS a served extension capability has to bind it
	// itself or every call answers "no pool for the extension runtime".
	compose.BindExtensionRuntime(e.Pool, e.Vault)
	return e
}

// grantDemoExtEndpointToAdmin gives the bootstrapped admin the RBAC object
// the synthetic verb gates on — an extension RBAC object is "held by no
// seeded role" by design (an operator grants it deliberately), so the
// human-works case needs it granted explicitly, the same way other suites
// rewrite a role's permissions directly over SQL.
func grantDemoExtEndpointToAdmin(t *testing.T, e *apptest.AppEnv) {
	t.Helper()
	if _, err := e.Owner.Exec(context.Background(),
		`UPDATE role SET permissions = jsonb_set(permissions, ARRAY['objects','ext_demoext_endpoint'],
			'{"create":true,"read":true,"update":true,"delete":true}'::jsonb, true)
		 WHERE key = 'admin'`); err != nil {
		t.Fatalf("granting ext_demoext_endpoint to the admin role: %v", err)
	}
}

// capRefusal is the slice of the RFC 7807 problem these cases assert on: the
// machine code a client branches on, plus the detail a human reads.
type capRefusal struct {
	Code   string `json:"code"`
	Detail string `json:"detail"`
}

// assertNothingStaged proves a refusal never reached the 🟡 admission gate: a
// staged approval is a durable authority object, so its absence is the
// observable difference between "refused outright" and "asked a human".
func assertNothingStaged(t *testing.T, e *apptest.AppEnv, what string) {
	t.Helper()
	var staged int
	if err := e.Owner.QueryRow(context.Background(), `SELECT count(*) FROM approval`).Scan(&staged); err != nil {
		t.Fatal(err)
	}
	if staged != 0 {
		t.Fatalf("%s staged %d approval(s) — a human-only refusal must never reach the approval gate", what, staged)
	}
}

// assertRefusalLeaksNothing holds the refusal body to both halves of the
// error bar: it names why the caller was refused, and it carries no
// operator-only detail.
func assertRefusalLeaksNothing(t *testing.T, detail, mustName string) {
	t.Helper()
	if !strings.Contains(detail, mustName) {
		t.Fatalf("refusal %q does not name %q", detail, mustName)
	}
	lower := strings.ToLower(detail)
	for _, leak := range []string{
		"select ", "insert ", "update ", "app_user", "workspace_id",
		"pgx", "apperrors", ".go:", "goroutine", "/users/", "internal/platform",
	} {
		if strings.Contains(lower, leak) {
			t.Fatalf("refusal %q leaks internals (%q)", detail, leak)
		}
	}
}

// TestAHumanOnlyExtensionOperationIsRefusedForAnAgentPrincipal proves the
// REST half: an Agent principal calling a human-only extension route is
// refused permission_denied, whatever its passport's scopes — a human-only
// verb carries none to check, because x-agent-access: human-only requests no
// agent authority at all.
func TestAHumanOnlyExtensionOperationIsRefusedForAnAgentPrincipal(t *testing.T) {
	e := bootWithDemoExt(t)
	apptest.BootstrapWorkspaceSession(t, e, "Demo Ext Human-Only", "demoext@fable.test", "Admin")

	bearer := apptest.PassportBearer(t, e, "drafting agent", "read", "write")

	var refusal capRefusal
	status := e.Call(t, "PUT", "/v1/ext/demoext/open", map[string]any{}, bearer, &refusal)
	if status != http.StatusForbidden || refusal.Code != "permission_denied" {
		t.Fatalf("agent demoext_open → %d %q, want 403 permission_denied (human-only)", status, refusal.Code)
	}
	if refusal.Code == "approval_required" {
		t.Fatalf("an agent reached the 🟡 gate on a human-only verb: %q", refusal.Detail)
	}
	assertNothingStaged(t, e, "refused demoext_open")
	assertRefusalLeaksNothing(t, refusal.Detail, "human-only")
}

// TestAHumanOnlyExtensionToolIsAbsentFromAnAgentsToolsList proves the MCP
// half: no human-only tool name is ever offered to an agent, whatever its
// passport's scopes — Registry.Offered excludes every HumanOnly spec for a
// PrincipalAgent caller before it ever reaches tools/list.
func TestAHumanOnlyExtensionToolIsAbsentFromAnAgentsToolsList(t *testing.T) {
	e := bootWithDemoExt(t, compose.WithMCPConnector())
	apptest.BootstrapWorkspaceSession(t, e, "Demo Ext Tool List", "demoext-list@fable.test", "Admin")

	token := apptest.MCPBearerToken(t, e, "broad agent", "read", "write")
	mcp := apptest.NewMCPClient(e, token)

	for _, name := range mcp.ListTools(t) {
		if name == "demoext_open" {
			t.Fatalf("tools/list offers %q — a human-only extension tool must never be listed to an agent", name)
		}
	}
}

// TestAHumanOnlyExtensionOperationStillWorksForAHuman proves the human
// session on the same route is unaffected: RequireHuman admits human/
// system/connector principals unchanged into the handler below.
func TestAHumanOnlyExtensionOperationStillWorksForAHuman(t *testing.T) {
	e := bootWithDemoExt(t)
	apptest.BootstrapWorkspaceSession(t, e, "Demo Ext Human Works", "demoext-human@fable.test", "Admin")
	grantDemoExtEndpointToAdmin(t, e)

	var result struct {
		Opened bool `json:"opened"`
	}
	if status := e.Call(t, "PUT", "/v1/ext/demoext/open", map[string]any{}, nil, &result); status != http.StatusOK {
		t.Fatalf("human demoext_open → %d, want 200", status)
	}
	if !result.Opened {
		t.Fatalf("handler result carries no opened=true: %+v", result)
	}
}
