// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
//gate:kind census H2

package gates

// What can be reached without credentials is a declared list, not whatever the
// routing code happens to allow.
//
// THE QUESTION THIS ANSWERS. "What is reachable unauthenticated?" is the first
// thing an auditor asks, and before this the answer was "read routes.go and
// work it out". Worse for the day-to-day: adding an unauthenticated route was
// one contact's decision in one file, and nothing noticed. A reviewer could not
// tell an intentional exception from an accidental one, because both look
// identical — a route registered without the session middleware.
//
// DERIVED, then compared. The set comes from the routing table itself: every
// mux registration whose handler does not pass through the session middleware.
// A hand-kept list alone would go stale the first time somebody forgot it, and
// a stale allowlist is worse than none because it reads as authoritative.
//
// TWO KINDS, because they are different claims and ADR-0013 leaves the
// distinction open. `openToAnyone` is genuinely unauthenticated — a probe, a
// metric, a discovery document, the login route that exists to mint the
// credential. `verifiesItsOwnCaller` is not: an inbound push endpoint proves
// its sender by other means (a Google-signed token, a shared-secret signature)
// and would be a defect if it did not. Collapsing them would let a route that
// forgot its verification hide among the ones that never needed one.

import (
	"go/ast"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const routingTable = "internal/compose/routes.go"

// routingPackage holds every file that may mount a route.
const routingPackage = "internal/compose"

// sessionMiddleware is the admission point. A handler expression that reaches
// it is gated; one that does not is below the gate.
const sessionMiddleware = "Middleware"

// openToAnyone are the paths served with no credential and no caller proof at
// all, each with why that is right.
var openToAnyone = gatekit.Waive(map[string]string{
	"/healthz": "the liveness probe. It answers before anything is wired, which is the whole point — a " +
		"probe that needed a credential could not report a process too broken to check one",
	"/readyz": "the readiness probe, same ground: the orchestrator asks it to decide whether to send " +
		"traffic, and it has no session to ask with",
	"GET /setup/status": "first-boot: says whether this installation has been claimed. It exists for the " +
		"state where no account exists yet, so requiring one is impossible by construction",
	"POST /setup/claim": "first-boot: mints the first account. The route that creates the credential " +
		"cannot require it",
	"/.well-known/oauth-authorization-server": "the OAuth discovery document a client reads BEFORE it " +
		"has a token, which is what discovery is for (RFC 8414)",
	"/.well-known/oauth-protected-resource":     "the same, for the protected-resource metadata (RFC 9728)",
	"/.well-known/oauth-protected-resource/mcp": "the MCP-scoped spelling of the same document",
	"GET /.well-known/security.txt": "the file a researcher reads to learn where to report a hole (RFC 9116). " +
		"It is served only when the deployment configured one, and it holds no installation data",
})

// verifiesItsOwnCaller are the paths that carry no session and are NOT open:
// each proves its sender by another mechanism, named here so a route that lost
// that mechanism is a visible change rather than a quiet one.
var verifiesItsOwnCaller = gatekit.Waive(map[string]string{
	"/metrics": "checks the configured metrics token as a bearer credential in constant time " +
		"(gateMetrics), and refuses every scrape when none is configured — a scraper is not a seat and " +
		"holds no login. Only an explicit --metrics-access=open serves it without one, for a port the " +
		"deployment declares contained",
	"/webhooks/gmail": "verifies a Google-signed OIDC token on the push notification (oidcverify.go). " +
		"The sender is proven; what it does not carry is one of OUR sessions",
	"/webhooks/graph": "the Microsoft Graph push, proven the same way by its own subscription secret",
	"/mcp": "the remote MCP edge. It mounts its own admission (mcpEdge) rather than the session " +
		"middleware, because an MCP caller presents a passport rather than a browser session",
	"/": "the SPA fallback: static assets and the index document, which are public by nature and " +
		"carry nothing of the installation's data",
})

// dynamicRegistrations are the registrations whose pattern is built at run time,
// keyed "file.go:function". The walk cannot read their path. Each is declared by
// where it is, with what stands in for the session or what gates it.
var dynamicRegistrations = gatekit.Waive(map[string]string{
	"extinbound.go:MountInboundEndpoints": "one signed POST edge for each endpoint a unit declares, at " +
		"/webhooks/ext/<unit>/<slug>. The core admits the request (workspace, body cap, freshness, rate) and " +
		"the unit proves the sender by its signature; TestEveryUnitThatDeclaresAnInboundEndpointChecksItsSignature " +
		"holds every unit to that call",
	"extroutes.go:MountExtensionRoutes": "not below the gate: it registers onto an inner mux that " +
		"extensionEdge wraps around the generated router INSIDE the session chain (routes.go), so an " +
		"extension operation is an authenticated, workspace-scoped call like any other /v1 route. The " +
		"wrapper cannot be seen from the registration, so it is declared here",
})

func TestEveryRouteBelowTheAuthGateIsDeclared(t *testing.T) {
	t.Parallel()

	below := routesBelowTheGate(t)
	if len(below) == 0 {
		t.Fatal("no route reads as below the auth gate — the routing table's shape changed and this " +
			"walk can no longer see it, which reports a clean answer to the one question it exists for")
	}
	for _, path := range below {
		open := openToAnyone.Waived(t, path)
		verified := verifiesItsOwnCaller.Waived(t, path)
		switch {
		case open && verified:
			t.Errorf("%s is declared both open and self-verifying — the two are different claims about "+
				"the same route, and a reader cannot tell which one holds", path)
		case open || verified:
		default:
			t.Errorf("%s is served WITHOUT the session middleware and is declared nowhere. Adding a route "+
				"below the auth gate is a security decision: put it in openToAnyone if it is genuinely "+
				"public, or in verifiesItsOwnCaller with the mechanism that proves its sender — either "+
				"way with the reason, so the next reviewer can tell an intentional exception from an "+
				"accidental one", path)
		}
	}
	// The other direction, and gatekit does it: a declaration matching no
	// ungated route is a standing permission nobody needs, and the next route
	// registered at that path would inherit it silently. AssertAllMatched is
	// also what holds the reasons to a standard — an empty one is a finding
	// there rather than something this walk has to re-check.
	openToAnyone.AssertAllMatched(t)
	verifiesItsOwnCaller.AssertAllMatched(t)
	dynamicRegistrations.AssertAllMatched(t)
}

// routesBelowTheGate reads every mux registration of the composition package and
// answers the patterns whose handler never reaches the session middleware.
//
// Every file, not the routing table alone. A route mounted from its own file is
// as reachable as one in routes.go. A census of one file reports a clean answer
// over the rest. A pattern built from a package constant is read through it.
// One built at run time is judged by where it is registered.
func routesBelowTheGate(t *testing.T) []string {
	t.Helper()
	dir := filepath.Join(repoRoot, "backend", routingPackage)
	paths, err := filepath.Glob(filepath.Join(dir, "*.go"))
	if err != nil {
		t.Fatal(err)
	}
	var files []*ast.File
	var names []string
	for _, path := range paths {
		if strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
			continue
		}
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		files = append(files, file)
		names = append(names, filepath.Base(path))
	}
	constants := gatekit.PackageStringConstants(t, dir)
	var below []string
	for i, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, ok := n.(*ast.CallExpr)
				if !ok || !isMuxRegistration(call) || len(call.Args) < 2 {
					return true
				}
				if reachesSessionMiddleware(call.Args[1]) {
					return true
				}
				if pattern, readable := gatekit.StringExpr(call.Args[0], constants, gatekit.FoldStrict); readable {
					below = append(below, pattern)
				} else if !dynamicRegistrations.Waived(t, names[i]+":"+fn.Name.Name) {
					t.Errorf("%s:%s registers a route below the auth gate whose pattern this walk cannot read, "+
						"so what it serves is unchecked. Declare it in dynamicRegistrations with the mechanism that proves its caller",
						names[i], fn.Name.Name)
				}
				return true
			})
		}
	}
	sort.Strings(below)
	return below
}

// isMuxRegistration reports whether a call registers a route on the mux.
func isMuxRegistration(call *ast.CallExpr) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || (sel.Sel.Name != "Handle" && sel.Sel.Name != "HandleFunc") {
		return false
	}
	recv, ok := sel.X.(*ast.Ident)
	return ok && recv.Name == "mux"
}

// reachesSessionMiddleware reports whether a handler expression passes through
// the session middleware anywhere in its wrapping.
//
// By NAME rather than by resolved identity, and the cost is the safe direction:
// a different Middleware reaching this walk would read as gated, so the check
// is paired with the declaration above — a route wrongly read as gated appears
// in neither list and the reverse comparison reports it as an entry serving no
// ungated route.
func reachesSessionMiddleware(handler ast.Expr) bool {
	found := false
	ast.Inspect(handler, func(n ast.Node) bool {
		if found {
			return false
		}
		if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == sessionMiddleware {
			found = true
		}
		return !found
	})
	return found
}
