// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// leadHandovers are the functions where a lead passes to somebody: promoted
// into a contact, claimed, given an owner, routed, or merged into another
// lead. Each asks the notice hold itself, so no door around it skips one.
//
// gatekit:fixture the lead handover functions, and what each one serves
var leadHandovers = map[string]string{
	"promotableLead":            "every promotion: REST, the agent tools, qualify, held and mail-driven promotion",
	"ClaimRecord":               "taking ownership",
	"ensureLeadUpdateAuthority": "an owner set by an update, a bulk reassignment or an assignment",
	"RouteLead":                 "automatic routing",
	"mergeLeadTx":               "a lead merged into another, by a human or an accepted merge proposal",
	"linkableLead":              "a lead linked to a company a human confirmed",
}

// The hold is one shared check, and every handover calls it.
func TestEveryLeadHandoverAsksTheNoticeHold(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	found := map[string]bool{}
	for _, name := range files {
		if strings.HasSuffix(name, "_test.go") {
			continue
		}
		src, err := os.ReadFile(name)
		if err != nil {
			t.Fatal(err)
		}
		file, err := parser.ParseFile(token.NewFileSet(), name, src, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil {
				continue
			}
			if _, listed := leadHandovers[fn.Name.Name]; !listed {
				continue
			}
			found[fn.Name.Name] = true
			if !callsTheHold(fn.Body) {
				t.Errorf("%s (%s) hands a lead on without asking the notice hold: a website lead "+
					"whose notice may still bounce would reach somebody", fn.Name.Name, leadHandovers[fn.Name.Name])
			}
		}
	}
	for name := range leadHandovers {
		if !found[name] {
			t.Errorf("%s is listed as a lead handover and no longer exists; point the list at what replaced it", name)
		}
	}
}

// callsTheHold reports whether body calls one of the two hold functions.
func callsTheHold(body *ast.BlockStmt) bool {
	calls := false
	ast.Inspect(body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, ok := call.Fun.(*ast.Ident); ok &&
			(ident.Name == "refuseWebsiteLeadNotYetTold" || ident.Name == "websiteLeadNoticePending") {
			calls = true
		}
		return !calls
	})
	return calls
}
