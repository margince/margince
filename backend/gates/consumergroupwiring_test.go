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
// So this reads the names the worker actually passes to its subscribe helpers
// out of its own source, and checks the catalog answers each one.

import (
	"go/ast"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/events"
)

// wantMinimumSubscribedLanes guards the way this gate fails silently. An
// extractor that stops recognising the call finds no names and reports PASS
// over an empty set, with no failing assertion to notice. Twenty-nine
// stand today. The floor sits below that, so removing a lane stays an ordinary
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
		t.Fatalf("only %d subscribe call(s) found in the worker, want at least %d — "+
			"the extractor lost its source rather than the worker losing its lanes",
			len(names), wantMinimumSubscribedLanes)
	}

	for name, where := range names {
		if !declared[name] {
			t.Errorf("%s subscribes %q, which the catalog does not declare — "+
				"the worker will log 'no such consumer group' and this lane will "+
				"deliver nothing, silently. Add it to events.Groups()", where, name)
		}
	}
}

// subscribedGroupNames reads every literal group name the worker hands a
// subscribe helper, keyed by name and valued by where it was found. Both
// directions of the catalog parity read subscriptions through it.
//
// Only string LITERALS are read. A name built at runtime cannot be checked
// here, so the gate fails on one rather than passing over it.
func subscribedGroupNames(t *testing.T) map[string]string {
	t.Helper()
	dir := filepath.Join(repoRoot, "backend", "cmd", "worker")
	entries, err := gatekit.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the worker package: %v", err)
	}
	files := map[string]*ast.File{}
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := gatekit.ParseFile(filepath.Join(dir, entry.Name()), 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", entry.Name(), err)
		}
		files[entry.Name()] = file
	}
	helpers := subscribeHelpers(files)
	if len(helpers) < 2 {
		t.Fatalf("found %d worker function(s) that resolve a group through catalogGroup, want runSubscriber "+
			"and runCoalescingSubscriber at least — the lanes are started some other way now, so teach "+
			"subscribeHelpers that way", len(helpers))
	}
	found := map[string]string{}
	for name, file := range files {
		ast.Inspect(file, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			ident, ok := call.Fun.(*ast.Ident)
			if !ok {
				return true
			}
			groupArg, isHelper := helpers[ident.Name]
			if !isHelper || len(call.Args) <= groupArg {
				return true
			}
			group, isLiteral := stringConst(call.Args[groupArg])
			if !isLiteral {
				t.Errorf("%s calls %s with a group name that is not a string literal, so neither direction "+
					"of the catalog parity can read it: pass the name as a literal", name, ident.Name)
				return true
			}
			found[group] = name
			return true
		})
	}
	return found
}

// subscribeHelpers derives the worker's subscribe helpers: each function that
// hands one of its own parameters to catalogGroup, mapped to that parameter's
// position. A new helper is then read without anyone naming it here.
func subscribeHelpers(files map[string]*ast.File) map[string]int {
	helpers := map[string]int{}
	for _, file := range files {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Name.Name == "catalogGroup" {
				continue
			}
			params := map[string]int{}
			position := 0
			for _, field := range fn.Type.Params.List {
				for _, param := range field.Names {
					params[param.Name] = position
					position++
				}
			}
			ast.Inspect(fn.Body, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok || len(call.Args) == 0 {
					return true
				}
				if callee, ok := call.Fun.(*ast.Ident); !ok || callee.Name != "catalogGroup" {
					return true
				}
				if arg, ok := call.Args[0].(*ast.Ident); ok {
					if at, isParam := params[arg.Name]; isParam {
						helpers[fn.Name.Name] = at
					}
				}
				return true
			})
		}
	}
	return helpers
}
