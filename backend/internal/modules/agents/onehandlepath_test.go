// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package agents

// Invoke's docblock says "There is no other path to a Handle in this package".
// This is what makes that a fact rather than a retelling.
//
// The claim is load-bearing and it is the kind that rots quietly. Everything
// that guards a tool call — the admission gate, the tier tightening, the
// approval redemption, the idempotency claim, the envelope seal, the cost
// share — sits on the ONE line that reaches mcp.Tool.Handle. A second call
// added anywhere in this package is a tool run with none of it, and it would
// compile, pass every test, and answer correctly. There is nothing about it to
// notice.
//
// Package-scoped on purpose, and the scope is the interesting half: compose has
// its own gate.Admit call (agentgate.go), so a tree-wide reading of this
// sentence is false. What is true is the reading the comment actually makes —
// in THIS package — and a gate that proved the wrong one would be worse than
// none.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"strings"
	"testing"
)

func TestOnlyOnePathReachesAToolHandle(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("reading the agents package: %v", err)
	}

	fset := token.NewFileSet()
	var sites []string
	files := 0
	for _, entry := range entries {
		name := entry.Name()
		if entry.IsDir() || !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		files++
		parsed, err := parser.ParseFile(fset, name, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		ast.Inspect(parsed, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != "Handle" {
				return true
			}
			sites = append(sites, fmt.Sprintf("%s:%d", name, fset.Position(call.Pos()).Line))
			return true
		})
	}

	// A census passes by finding nothing, which is also what it does over a
	// directory it failed to read. The floor tells the two apart.
	if files < 10 {
		t.Fatalf("the walk found %d production files in the agents package, so it is reading the wrong "+
			"directory and would report the clean word over anything", files)
	}
	if len(sites) != 1 {
		t.Errorf("mcp.Tool.Handle is called from %d places in this package:\n\t%s\n\n"+
			"Invoke's docblock says there is no other path to a Handle here, and everything that guards a "+
			"tool call — admission, tier tightening, approval redemption, the idempotency claim, the "+
			"envelope seal, the cost share — hangs off that one line. A second call runs a tool with none "+
			"of it and looks exactly like a correct answer. Route it through Invoke, or move the claim",
			len(sites), strings.Join(sites, "\n\t"))
	}
}
