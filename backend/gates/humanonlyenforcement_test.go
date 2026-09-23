// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H1

package gates

// The declaration reaches the thing that enforces it.
//
// `x-agent-access: human-only` in api/crm.yaml is a sentence in a document.
// What refuses an agent is agentGate, and it does not read the document: it
// reads agentPolicies, a Go map generated from it. One generator sits between
// the annotation and the refusal, and this compares its input to its output.
//
// `make drift` cannot. It regenerates the table and diffs the result against
// the committed file, which proves the file is current and nothing else — a
// generator that stopped carrying the annotation emits a smaller table, the
// diff is clean, and drift passes. Every route that lost its class is then an
// ordinary agent-readable route, refused by nothing, with the annotation still
// in the contract saying otherwise. That failure is invisible from either side
// alone.
//
// agentgateinstalled_test.go holds the other half — that the middleware is
// wrapped around the routes at all. Between them: the gate is installed, and
// the gate knows which operations it is for.
//
// BOTH DIRECTIONS, because they fail differently. An operation the contract
// marks and the table misses is a route standing open. An operation the table
// marks and the contract does not is a route closed to agents for no reason a
// reader can find — the contract is what a caller is entitled to read, and a
// refusal it does not predict is as much a defect as one it predicts wrongly.
//
// WHAT THIS CANNOT SEE: whether the gate, holding the right table, then decides
// correctly. That is agentgateredeem_test.go's subject.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/compose"
)

// humanOnlyAccess is the annotation's value in the contract. The generated
// table's own spelling of it is compose's (accessHumanOnly), and the two being
// the same string is what makes the comparison below meaningful rather than a
// translation this gate would own a copy of.
const humanOnlyAccess = "human-only"

func TestEveryHumanOnlyOperationReachesTheGate(t *testing.T) {
	t.Parallel()

	declared := humanOnlyOperationsInContract(t)
	// Counted before anything is compared. Two empty sets agree, so a walk that
	// stopped reaching the operations — a renamed annotation, a restructured
	// document — would otherwise report the strongest possible PASS.
	if len(declared) == 0 {
		t.Fatal("no operation in api/crm.yaml declares x-agent-access: human-only, so this comparison " +
			"proved nothing — either the annotation was renamed or the walk no longer reaches the operations")
	}

	enforced := compose.AgentHumanOnlyRoutes()
	if len(enforced) == 0 {
		t.Fatal("the generated agentPolicies table refuses no operation as human-only, so the annotation " +
			"reaches nothing that enforces it")
	}

	prefixes := map[string]bool{}
	for _, op := range sortedKeys(declared) {
		claimed := enforced[op]
		if len(claimed) > 1 {
			t.Errorf("the generated table refuses %s as %s under %d routes: %v.\n\tOne operation is one route "+
				"here; two means the generator emitted an id twice and this comparison can only speak for one "+
				"of them.", op, humanOnlyAccess, len(claimed), claimed)
			continue
		}
		if len(claimed) == 0 {
			t.Errorf("%s declares x-agent-access: %s in api/crm.yaml and the generated agentPolicies table "+
				"does not carry it.\n\tagentGate refuses an agent from that table, so the route is open to any "+
				"passport-bearing agent while the contract says it is not. Regenerate (make gen); if it is still "+
				"missing, tools/gen-agentpolicy stopped carrying the annotation.", op, humanOnlyAccess)
			continue
		}
		route := claimed[0]
		prefix, matched := basePathBetween(route, declared[op])
		if !matched {
			t.Errorf("%s is human-only at %q in api/crm.yaml and the generated table refuses it under %q.\n\t"+
				"agentGate looks a call up by ROUTE and only the operation carries the annotation, so a route "+
				"on the wrong policy refuses one call and admits another while both operation ids are still "+
				"present.", op, declared[op], route)
			continue
		}
		prefixes[prefix] = true
	}
	for op := range enforced {
		if _, ok := declared[op]; !ok {
			t.Errorf("the generated agentPolicies table refuses %s as %s and api/crm.yaml does not declare "+
				"it.\n\tA caller reads the contract, so a refusal it cannot predict is as much a defect as a "+
				"wrong one. Either annotate the operation or stop refusing it.", op, humanOnlyAccess)
		}
	}
	// ONE prefix, whatever it is. The generator prepends a base path the
	// contract's paths do not carry; asking that every route agrees on it binds
	// the shape without this gate deciding what it should be, and catches a
	// generator that started prefixing some routes and not others.
	if len(prefixes) > 1 {
		t.Errorf("the generated routes do not share one base path: %v.\n\tagentGate keys on the chi pattern the "+
			"contract router registered, so a route carrying a different base is one the gate never matches.",
			sortedKeys(prefixes))
	}
}

// basePathBetween reports the base path the generated route carries in front of
// the contract's own, and whether the two name the same operation at all.
//
// Both sides read "METHOD path", and only the path differs: the generator
// prepends a base the contract's paths do not carry. So the method must match
// exactly and the path must be a suffix — which binds the route to the
// operation without this gate deciding what the base should be.
func basePathBetween(route, declared string) (string, bool) {
	routeMethod, routePath, ok := strings.Cut(route, " ")
	if !ok {
		return "", false
	}
	declaredMethod, declaredPath, ok := strings.Cut(declared, " ")
	if !ok || routeMethod != declaredMethod {
		return "", false
	}
	return strings.CutSuffix(routePath, declaredPath)
}

// humanOnlyOperationsInContract reads the operations the contract marks
// human-only, as operationId → the "METHOD /path" it is declared at.
//
// Keyed by operationId, which is what the generated table records alongside
// each route, so neither side has to rebuild the other's key; valued by the
// route so the caller can bind the two rather than trust either alone.
func humanOnlyOperationsInContract(t *testing.T) map[string]string {
	t.Helper()
	doc := loadContract(t)
	paths, ok := doc["paths"].(map[string]any)
	if !ok {
		t.Fatal("api/crm.yaml carries no paths, so this census would clear every operation by reading none")
	}
	declared := map[string]string{}
	for path, item := range paths {
		operations, ok := item.(map[string]any)
		if !ok {
			continue
		}
		refusePathLevelAgentAccess(t, path, operations)
		for method, raw := range operations {
			if !httpMethods[method] {
				continue
			}
			op, ok := raw.(map[string]any)
			if !ok || op["x-agent-access"] != humanOnlyAccess {
				continue
			}
			name, _ := op["operationId"].(string)
			if name == "" {
				t.Errorf("%s %s is human-only and carries no operationId, so nothing can tie it to the "+
					"table that enforces the annotation", method, path)
				continue
			}
			// Keyed by operationId, which operationiduniqueness_test.go holds
			// unique over this same document — so one id is one operation here
			// and this map cannot quietly drop a route by overwriting it.
			declared[name] = strings.ToUpper(method) + " " + path
		}
	}
	return declared
}

// refusePathLevelAgentAccess fails when `x-agent-access` is written on a PATH
// ITEM rather than on an operation.
//
// It would be silent otherwise, and silent in the worst direction. Nothing
// reads it there: tools/gen-agentpolicy walks the path item's keys, skips
// every one that is not an http method, and decodes the annotation off the
// operation node — so a path-level declaration generates no table entry and
// agentGate refuses nobody. Both censuses over this document read it the same
// way, so both would count one operation fewer and report PASS over a route
// that declares a class it does not have.
//
// Shared by this gate and TestEveryHumanOnlyOperationNarrowsItsTransport,
// which walk the same document asking different questions of it. One helper
// rather than two, because a check that is only in one of them is exactly the
// hole it exists to close.
//
// OpenAPI has no inheritance for a vendor extension, so this is a refusal
// rather than a fix: there is no correct meaning to give the declaration, and
// pretending there is would put a second answer beside the generator's.
func refusePathLevelAgentAccess(t *testing.T, path string, item map[string]any) {
	t.Helper()
	if _, present := item["x-agent-access"]; !present {
		return
	}
	t.Errorf("%s declares x-agent-access on the PATH ITEM.\n\tNothing reads it there — "+
		"tools/gen-agentpolicy takes the annotation off the OPERATION, so this declares a class the "+
		"gate never enforces and shrinks what every census over this document can see. Move it onto "+
		"each operation it is meant to bind.", path)
}
