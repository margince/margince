// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//go:build integration

package integration

// openchannel's 7 operations declare x-agent-access: human-only rather than
// x-mcp-tool — the end-to-end proof that DESIGN.md's mechanism actually
// closes every surface an agent could reach it through: REST (via
// agents.Registry.InvokeServing's RequireHuman check, since extension routes
// are never behind agentGate — extroutes.go mounts them onto
// registry.Invoke directly), and MCP tools/list (via Offered's HumanOnly
// filter). A human session is unaffected on both routes.
//
// The composed extension set (openchannel included) is registered here via
// the generated "composition" package rather than a synthetic fixture, the
// same call cmd/api/main.go makes at boot — apptest.SetupApp's harness never
// calls compose.RegisterExtensions itself, so a suite whose subject IS an
// extension route has to. It resolves to the real generated module only under
// the composed go.work (`make test-it`/`make test-integration`); a bare `go
// test` here would 404 on every /ext/ route, which is exactly the signal that
// this suite needs the composed lane.

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
	"github.com/margince/margince/backend/internal/compose/integration/apptest"
	"github.com/margince/margince/composition"
)

// registerOpenchannelOnce guards compose.RegisterExtensions: it is a
// package-level, process-wide reconciliation (extensions.go's own doc) —
// jurisdiction.Register beneath it refuses a second registration of the same
// pack outright — so every test in this binary that needs the composed set
// shares one call rather than each racing to be first.
var registerOpenchannelOnce sync.Once

// bootWithOpenchannel registers the real composed extension set (which
// includes openchannel) into the core registries before booting the app
// harness — this must run before compose.New assembles anything that reads
// it, and apptest.SetupApp's harness never calls it itself.
func bootWithOpenchannel(t *testing.T, opts ...compose.Option) *apptest.AppEnv {
	t.Helper()
	var registerErr error
	registerOpenchannelOnce.Do(func() {
		registerErr = compose.RegisterExtensions(composition.Extensions(), composition.Verbs(), composition.Jobs())
	})
	if registerErr != nil {
		t.Fatalf("registering the composed extension set: %v", registerErr)
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

// grantOpenchannelEndpointToAdmin gives the bootstrapped admin the one RBAC
// object openchannel_open gates on. DESIGN.md's own contract comment says
// this object is "held by no seeded role" — an operator grants it
// deliberately — so the human-works case needs it granted explicitly, the
// same way other suites rewrite a role's permissions directly
// (webhooks_integration_test.go's dropObjectReadFromEveryRole is the sibling
// pattern, the other direction).
func grantOpenchannelEndpointToAdmin(t *testing.T, e *apptest.AppEnv) {
	t.Helper()
	if _, err := e.Owner.Exec(context.Background(),
		`UPDATE role SET permissions = jsonb_set(permissions, ARRAY['objects','ext_openchannel_endpoint'],
			'{"create":true,"read":true,"update":true,"delete":true}'::jsonb, true)
		 WHERE key = 'admin'`); err != nil {
		t.Fatalf("granting ext_openchannel_endpoint to the admin role: %v", err)
	}
}

// TestOpenchannelOpenIsRefusedForAnAgentPrincipal proves the REST half: an
// Agent principal calling the same route a human uses is refused
// permission_denied, whatever its passport's scopes — openchannel_open
// carries none to check, because x-agent-access: human-only requests no
// agent authority at all.
func TestOpenchannelOpenIsRefusedForAnAgentPrincipal(t *testing.T) {
	e := bootWithOpenchannel(t)
	apptest.BootstrapWorkspaceSession(t, e, "Openchannel Human-Only", "openchannel@fable.test", "Admin")

	bearer := apptest.PassportBearer(t, e, "drafting agent", "read", "write")

	var refusal capRefusal
	status := e.Call(t, "PUT", "/v1/ext/openchannel/endpoint", AnyMap{}, bearer, &refusal)
	if status != http.StatusForbidden || refusal.Code != "permission_denied" {
		t.Fatalf("agent openchannel_open → %d %q, want 403 permission_denied (human-only)", status, refusal.Code)
	}
	if refusal.Code == "approval_required" {
		t.Fatalf("an agent reached the 🟡 gate on a human-only verb: %q", refusal.Detail)
	}
	assertNothingStaged(t, e, "refused openchannel_open")
	assertRefusalLeaksNothing(t, refusal.Detail, "human-only")
}

// TestOpenchannelToolsAreAbsentFromAnAgentsToolsList proves the MCP half: no
// openchannel_* name is ever offered to an agent, whatever its passport's
// scopes — Registry.Offered excludes every HumanOnly spec for a
// PrincipalAgent caller before it ever reaches tools/list.
func TestOpenchannelToolsAreAbsentFromAnAgentsToolsList(t *testing.T) {
	e := bootWithOpenchannel(t, compose.WithMCPConnector())
	apptest.BootstrapWorkspaceSession(t, e, "Openchannel Tool List", "openchannel-list@fable.test", "Admin")

	token := apptest.MCPBearerToken(t, e, "broad agent", "read", "write")
	mcp := apptest.NewMCPClient(e, token)

	for _, name := range mcp.ListTools(t) {
		if strings.HasPrefix(name, "openchannel_") {
			t.Fatalf("tools/list offers %q — a human-only extension tool must never be listed to an agent", name)
		}
	}
}

// TestOpenchannelOpenStillWorksForAHuman proves the human session on the same
// route is unaffected: RequireHuman admits human/system/connector principals
// unchanged into the handler below.
func TestOpenchannelOpenStillWorksForAHuman(t *testing.T) {
	e := bootWithOpenchannel(t)
	apptest.BootstrapWorkspaceSession(t, e, "Openchannel Human Works", "openchannel-human@fable.test", "Admin")
	grantOpenchannelEndpointToAdmin(t, e)

	var endpoint struct {
		ID  string `json:"id"`
		Ref string `json:"ref"`
	}
	if status := e.Call(t, "PUT", "/v1/ext/openchannel/endpoint", AnyMap{}, nil, &endpoint); status != http.StatusOK {
		t.Fatalf("human openchannel_open → %d, want 200", status)
	}
	if endpoint.ID == "" || endpoint.Ref == "" {
		t.Fatalf("opened endpoint carries no id/ref: %+v", endpoint)
	}
}
