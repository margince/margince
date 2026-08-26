// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package people

import (
	"go/ast"
	"go/token"
	"sort"
	"strings"
	"testing"
)

// Two claims that a struct literal cannot hold on its own.
//
// The promote preview answers "who is this lead already?" and the promotion
// then acts on that answer. They agree only while the CANDIDATE is derived once
// — two literals naming the same fields still disagree when the values fed into
// them are worked out differently, and the preview would name a person the
// promotion does not land on. That is worse than a plain wrong answer, because
// the preview is what a human read before agreeing.
//
// A lead update carries a gesture JSON erases: null means clear the override,
// absent means leave it. Only LeadUpdateRequest records the difference, so a
// transport decoding the bare contract type reads a clear as a no-op — the
// caller's request succeeds and the override they asked to remove is still
// there.

// candidateType is the ladder's input, built in one place or not at all.
const candidateType = "PersonCandidate"

// candidateBuilder is where that one place is.
const candidateBuilder = "leadPersonCandidate"

func TestTheLadderCandidateIsDerivedInOnePlace(t *testing.T) {
	builders := map[string]bool{}
	forEachModuleFile(t, func(_ string, fset *token.FileSet, file *ast.File) {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Name == nil {
				continue
			}
			// From a LEAD specifically. The ladder matches candidates built
			// from several sources — a channel identity, a resolve, a manual
			// dedupe — and those are different derivations of a different
			// thing. The claim is about the one the preview and the promotion
			// share, and its input is what identifies it.
			if takesA(fn, "crmcontracts.Lead") && buildsA(fn.Body, candidateType) {
				builders[fn.Name.Name] = true
			}
		}
	})
	if !builders[candidateBuilder] {
		t.Fatalf("%s does not build a %s, so this gate is watching the wrong function and the "+
			"derivation it was meant to hold is somewhere else now", candidateBuilder, candidateType)
	}
	if len(builders) == 1 {
		return
	}
	others := make([]string, 0, len(builders))
	for name := range builders {
		if name != candidateBuilder {
			others = append(others, name)
		}
	}
	sort.Strings(others)
	t.Errorf("a %s is derived from a lead by %s as well as by %s.\n\nThe preview and the "+
		"promotion agree only while "+
		"the candidate is derived once: same fields, different working-out, and the preview names "+
		"a person the promotion does not land on — after a human read the preview and agreed. "+
		"Take %s.", candidateType, strings.Join(others, ", "), candidateBuilder, candidateBuilder)
}

// contractLeadUpdate is the generated request type, which drops the
// null-vs-absent gesture on decode.
const contractLeadUpdate = "UpdateLeadRequest"

// leadUpdateWrapper is the type that keeps it.
const leadUpdateWrapper = "LeadUpdateRequest"

func TestEveryLeadUpdateDecodeKeepsTheNullGesture(t *testing.T) {
	wrapperSeen, decodes := false, 0
	forEachModuleFile(t, func(name string, fset *token.FileSet, file *ast.File) {
		ast.Inspect(file, func(n ast.Node) bool {
			if spec, ok := n.(*ast.TypeSpec); ok && spec.Name.Name == leadUpdateWrapper {
				wrapperSeen = true
				return true
			}
			decl, ok := n.(*ast.DeclStmt)
			if !ok {
				return true
			}
			gen, ok := decl.Decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				return true
			}
			for _, spec := range gen.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok || vs.Type == nil {
					continue
				}
				switch typeText(vs.Type) {
				case leadUpdateWrapper:
					decodes++
				case "crmcontracts." + contractLeadUpdate:
					t.Errorf("%s declares a decode target of the bare %s.\n\nJSON erases the "+
						"difference between null and absent on a pointer field, and only %s "+
						"records it — decoded this way, a caller asking to CLEAR an override "+
						"gets a successful response and keeps the override.",
						fset.Position(vs.Pos()), contractLeadUpdate, leadUpdateWrapper)
				}
			}
			return true
		})
	})
	if !wrapperSeen {
		t.Fatalf("this module declares no %s, so the gesture has no keeper and this gate judged "+
			"nothing", leadUpdateWrapper)
	}
	if decodes < 2 {
		t.Errorf("%d transport(s) decode into %s; the claim is that BOTH the handler and the "+
			"provider do, and one transport cannot drift from itself", decodes, leadUpdateWrapper)
	}
}

// takesA reports whether fn accepts the named type as a parameter — which is
// what makes a body's reads and literals be ABOUT that type rather than about
// some other value that happens to share a field or a shape. Shared by the
// gates in this package that identify a function by what it is handed.
func takesA(fn *ast.FuncDecl, typeName string) bool {
	if fn.Type.Params == nil {
		return false
	}
	for _, param := range fn.Type.Params.List {
		if typeText(param.Type) == typeName {
			return true
		}
	}
	return false
}

// buildsA reports whether body contains a composite literal of the named type.
func buildsA(body *ast.BlockStmt, typeName string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if !ok || lit.Type == nil {
			return true
		}
		// An empty literal is a zero value on an error return, not a candidate
		// anyone matches on; counting it would report every error path as a
		// second derivation.
		if typeText(lit.Type) == typeName && len(lit.Elts) > 0 {
			found = true
		}
		return !found
	})
	return found
}
