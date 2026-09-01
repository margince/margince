// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
//gate:kind parity H3

package gates

// Every lane the worker starts is a group the catalog declares.
//
// runSubscriber looks its group name up in events.Groups() and, finding
// nothing, logs "no such consumer group" and returns. The worker still boots,
// every other lane still runs, and the missing one delivers nothing forever —
// no panic, no failed health check, no error a deploy would notice. The only
// symptom is a feature that quietly does not happen, which is exactly how a
// consumer wired without its catalog entry reached a branch that passed every
// other gate: the integration tests called HandleEvent directly and never went
// near the bus.
//
// So this reads the names the worker actually passes to runSubscriber out of
// its own source, and checks the catalog answers each one.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/events"
)

// wantMinimumSubscribedLanes guards the way this gate fails silently: an
// extractor that stops recognising the call finds no names, reports PASS over
// an empty set, and there is no failing assertion to notice. Seventeen stand
// today; the floor sits below that so removing a lane stays an ordinary
// change and only a collapse is a finding.
const wantMinimumSubscribedLanes = 12

func TestEveryWorkerLaneNamesADeclaredConsumerGroup(t *testing.T) {
	t.Parallel()

	declared := map[string]bool{}
	for _, group := range events.Groups() {
		declared[group.Name] = true
	}
	if len(declared) == 0 {
		t.Fatal("the consumer-group catalog is empty, so this gate would admit anything")
	}

	names := subscribedGroupNames(t)
	if len(names) < wantMinimumSubscribedLanes {
		t.Fatalf("only %d runSubscriber call(s) found in the worker, want at least %d — "+
			"the extractor lost its source rather than the worker losing its lanes",
			len(names), wantMinimumSubscribedLanes)
	}

	for name, where := range names {
		if !declared[name] {
			t.Errorf("%s subscribes %q, which the catalog does not declare — "+
				"runSubscriber will log 'no such consumer group' and this lane will "+
				"deliver nothing, silently. Add it to events.Groups()", where, name)
		}
	}
}

// subscribedGroupNames reads every literal group name the worker hands
// runSubscriber, keyed by name and valued by where it was found.
//
// Only string LITERALS are read. A name built at runtime cannot be checked
// here, and the gate says so by failing its census rather than by passing over
// one quietly — a computed name would simply be absent, which is why the
// minimum above exists.
func subscribedGroupNames(t *testing.T) map[string]string {
	t.Helper()
	dir := filepath.Join(repoRoot, "backend", "cmd", "worker")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the worker package: %v", err)
	}
	found := map[string]string{}
	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, entry.Name()), nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", entry.Name(), err)
		}
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			ident, ok := call.Fun.(*ast.Ident)
			if !ok || ident.Name != "runSubscriber" {
				return true
			}
			// runSubscriber(ctx, rdb, groupName, handler, log, minIdle) — the
			// name is the third argument. Positional rather than by keyword
			// because Go has no keyword arguments; a signature change breaks
			// this loudly through the census floor above.
			const groupNameArg = 2
			if len(call.Args) <= groupNameArg {
				return true
			}
			literal, ok := call.Args[groupNameArg].(*ast.BasicLit)
			if !ok || literal.Kind != token.STRING {
				return true
			}
			name, err := strconv.Unquote(literal.Value)
			if err != nil {
				return true
			}
			found[name] = entry.Name()
			return true
		})
	}
	return found
}
