// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// Every event consumer is actually subscribed.
//
// A consumer is a type in compose/ with a HandleEvent method. It does nothing
// until a starter in cmd/worker/ hands it to runSubscriber under a registered
// group name — and an unsubscribed one fails in the worst way available: it
// compiles, it has tests, it is wired into nothing, and the surface it feeds
// reads EMPTY rather than broken. An empty privacy queue says the workspace
// owes nobody a disclosure; an empty introduction projection says nobody ever
// replied. Both are answers, and both are wrong.
//
// This shipped once. NewNoticeCaseOpen was written, tested through its own
// HandleEvent, and reviewed twice before anybody noticed cmd/worker never
// called it — the integration test constructed the consumer by hand, so the
// missing wiring was invisible from inside the test that was supposed to prove
// the feature worked.

import (
	"go/ast"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestEveryEventConsumerIsSubscribed(t *testing.T) {
	t.Parallel()

	consumers := composeEventConsumers(t)
	// Under-recognition is the one way this must not fail: a walk that found
	// nothing would loop zero times and report PASS over an unsubscribed tree.
	if len(consumers) < 3 {
		t.Fatalf("found %d event consumers in internal/compose, want at least the three "+
			"long-standing ones: the census has stopped seeing its subject", len(consumers))
	}

	subscribed := subscribedConsumers(t)
	for _, consumer := range consumers {
		if !subscribed[consumer] {
			t.Errorf("compose.%s has a HandleEvent method and nothing REACHABLE FROM boot.go "+
				"subscribes it. An unsubscribed consumer does not fail — the surface it feeds "+
				"simply reads empty, which is an answer and a wrong one. Add a starter beside "+
				"cmd/worker/introadvance.go, register its group in "+
				"internal/shared/kernel/events/groups.go, and CALL the starter from boot.go",
				consumer)
		}
	}
}

// subscribedConsumers names every consumer reachable from a starter that
// boot.go actually calls.
//
// The call chain, not a name anywhere in the directory. A starter that exists
// and is never called leaves the consumer exactly as dead as no starter at all,
// and matching on the name alone would certify it — which is how this gate
// would have passed over the very defect it was written for. Comments and test
// files are excluded for the same reason: neither subscribes anything.
func subscribedConsumers(t *testing.T) map[string]bool {
	t.Helper()
	dir := filepath.Join(moduleRoot(t), "cmd", "worker")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading cmd/worker: %v", err)
	}

	// Which starters boot.go calls, and what each starter names.
	called := map[string]bool{}
	names := map[string][]string{}
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file := parseModuleFile(t, filepath.Join(dir, name))
		for _, decl := range file.Decls {
			fn, isFn := decl.(*ast.FuncDecl)
			if !isFn || fn.Body == nil {
				continue
			}
			for _, callee := range calledFunctions(fn.Body) {
				if name == "boot.go" {
					called[callee] = true
				}
				names[fn.Name.Name] = append(names[fn.Name.Name], callee)
			}
		}
	}
	// A consumer is subscribed when a starter boot.go calls constructs it.
	out := map[string]bool{}
	for starter, callees := range names {
		if !called[starter] {
			continue
		}
		for _, callee := range callees {
			out[strings.TrimPrefix(callee, "New")] = true
		}
	}
	return out
}

// calledFunctions names every plain or package-qualified function called in a
// body, as a bare identifier.
func calledFunctions(body *ast.BlockStmt) []string {
	var out []string
	ast.Inspect(body, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall {
			return true
		}
		switch fn := call.Fun.(type) {
		case *ast.Ident:
			out = append(out, fn.Name)
		case *ast.SelectorExpr:
			out = append(out, fn.Sel.Name)
		}
		return true
	})
	return out
}

// composeEventConsumers names every type in internal/compose declaring a
// HandleEvent method.
func composeEventConsumers(t *testing.T) []string {
	t.Helper()
	dir := filepath.Join(moduleRoot(t), "internal", "compose")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading internal/compose: %v", err)
	}
	var out []string
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file := parseModuleFile(t, filepath.Join(dir, name))
		for _, decl := range file.Decls {
			fn, isFn := decl.(*ast.FuncDecl)
			if !isFn || fn.Name.Name != "HandleEvent" {
				continue
			}
			if receiver := receiverTypeName(fn); receiver != "" {
				out = append(out, receiver)
			}
		}
	}
	return out
}
