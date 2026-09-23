// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H1

package gates

// `x-agent-access: human-only` is enforced by ONE line, and this is what holds
// it there.
//
// The annotation is a declaration; what refuses an agent is agentGate, an
// operation middleware wrapped around every route the contract router serves.
// Inside it, `prepareAgentGate` refuses a mutating human-only route and
// `refusedAsHumanOnly` refuses a reading one — centrally, from the generated
// agentPolicies table, so a handler does not have to ask and almost none do.
//
// Delete that one entry from the Middlewares slice and every human-only route
// in the product opens to any passport-bearing agent. Nothing else fails: the
// table is still generated and still correct, the annotations are still in the
// contract, the handlers still compile, and the ~400 routes that rely on the
// middleware rather than an in-handler call are reachable. `make check` stays
// green.
//
// That is the shape of a single point of failure with no alarm on it, which is
// what this test is. It asserts the wiring and nothing else — whether the gate
// then decides correctly is agentgateredeem_test.go's subject, and that the
// table carries every operation the contract annotates is
// humanonlyenforcement_test.go's. Not `make drift`, which regenerates the table
// and diffs it: that proves the committed file is current, and a generator that
// stopped carrying the annotation would emit a smaller table the diff would
// accept.
//
// The session middleware one file over has belowtheauthgate_test.go asking the
// same shape of question about the same file. Two gates rather than one because
// they are two admissions: that one answers "may this caller be here without
// credentials", this one answers "may this caller be an agent".

import (
	"go/ast"
	"go/parser"
	"go/token"
	"testing"
)

// theAgentGate is the middleware constructor, and contractRouter is the call
// whose middleware list must carry it.
const (
	theAgentGate   = "agentGate"
	contractRouter = "HandlerWithOptions"
)

func TestTheAgentGateIsWrappedAroundEveryContractRoute(t *testing.T) {
	t.Parallel()

	file, err := parser.ParseFile(token.NewFileSet(), routingTable, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", routingTable, err)
	}

	var routerFound, gateFound bool
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || sel.Sel.Name != contractRouter {
			return true
		}
		routerFound = true
		// The middlewares arrive inside the options literal, as a slice of
		// calls. Any mention of agentGate under this call is the wiring: a
		// name that reaches the router's options cannot be there for anything
		// else, and asking more precisely would mean pinning the option
		// struct's field name, which is generated and not ours.
		ast.Inspect(call, func(inner ast.Node) bool {
			if id, ok := inner.(*ast.Ident); ok && id.Name == theAgentGate {
				gateFound = true
			}
			return true
		})
		return true
	})

	if !routerFound {
		t.Fatalf("%s registers no %s call — this gate is reading for a wiring that has moved, "+
			"and a gate that cannot find its subject passes over anything", routingTable, contractRouter)
	}
	if !gateFound {
		t.Errorf("%s builds the contract router without %s in its middlewares.\n"+
			"\tEvery x-agent-access: human-only route is refused by that middleware and by nothing else — "+
			"about four hundred of them make no in-handler check at all. Without it they answer any agent "+
			"that presents a passport, and no other test in this repository fails.", routingTable, theAgentGate)
	}
}
