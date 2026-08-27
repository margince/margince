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

// A third derivation lives one field away from the two above, and it fails in
// the opposite direction: not two answers to who the lead is, but two readings
// of whether it is anybody.
//
// The promotion refuses a lead with no identity, and the ladder candidate works
// out the name to match on. Both turn on the same question — what is this lead
// called — and while each answers it for itself, the guard admits leads the
// candidate cannot name. `FullName != nil` is true of a full_name that is
// present and empty, so a lead carrying one and no email clears the guard and
// promotes into a person with no name at all: a row nobody searching for that
// person will ever match, created by a verb whose whole job is to name them.

// identityRefusal is the refusal a lead with nothing to be called earns.
const identityRefusal = "PromoteNeedsIdentityError"

// leadNaming is the one place a lead's name is worked out.
const leadNaming = "leadIdentityName"

// leadNameField is the field whose present-but-empty reading is the defect. A
// function in the corpus reading it directly has started a second answer,
// whatever it then does with it.
const leadNameField = "FullName"

func TestTheIdentityGuardAndTheCandidateReadOneName(t *testing.T) {
	var guards, builders int
	forEachModuleFile(t, func(name string, fset *token.FileSet, file *ast.File) {
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Body == nil || fn.Name == nil || fn.Name.Name == leadNaming {
				continue
			}
			// Both halves are found by what they DO — one refuses a lead for
			// having no identity, the other builds the candidate the ladder
			// matches on — so a third function joining either half is judged
			// the day it is written rather than the day somebody remembers
			// this test.
			guard := refusesWith(fn.Body, identityRefusal)
			builder := takesA(fn, "crmcontracts.Lead") && buildsA(fn.Body, candidateType)
			if !guard && !builder {
				continue
			}
			if guard {
				guards++
			}
			if builder {
				builders++
			}
			if !callsFunc(fn, leadNaming) {
				t.Errorf("%s (%s) decides what a lead is called without %s.\n\nThe guard and the "+
					"candidate agree on whether a lead has an identity only while ONE function "+
					"answers it. Take %s.", fn.Name.Name, name, leadNaming, leadNaming)
			}
			if pos := readsField(fn.Body, leadNameField); pos.IsValid() {
				t.Errorf("%s reads .%s directly at %s.\n\nThat is the second reading: `!= nil` and "+
					"`== \"\"` disagree about a full_name that is present and empty, and the lead "+
					"that falls between them promotes into a person with no name. Read %s.",
					fn.Name.Name, leadNameField, fset.Position(pos), leadNaming)
			}
		}
	})
	// Either half at zero means this gate examined a population that no longer
	// exists — the refusal renamed, the candidate moved — and a gate holding
	// nothing reports exactly what a gate holding everything does.
	if guards == 0 {
		t.Fatalf("no function builds a %s, so nothing in this package refuses a lead for having "+
			"no identity and this gate is watching a rule that has moved", identityRefusal)
	}
	if builders == 0 {
		t.Fatalf("no function builds a %s from a lead, so this gate is watching a derivation "+
			"that has moved", candidateType)
	}
}

// refusesWith reports whether body constructs the named error type. Unlike
// buildsA it counts an EMPTY literal: a sentinel error carries its meaning in
// its type, and `&PromoteNeedsIdentityError{}` is the whole refusal.
func refusesWith(body *ast.BlockStmt, typeName string) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		lit, ok := n.(*ast.CompositeLit)
		if ok && lit.Type != nil && typeText(lit.Type) == typeName {
			found = true
		}
		return !found
	})
	return found
}

// callsFunc reports whether fn calls the named package-level function. Plain
// identifiers only: a method of the same name on some other receiver answers a
// different question, and counting it would report a derivation that is not
// shared.
func callsFunc(fn *ast.FuncDecl, name string) bool {
	found := false
	ast.Inspect(fn.Body, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		if ident, ok := call.Fun.(*ast.Ident); ok && ident.Name == name {
			found = true
		}
		return !found
	})
	return found
}

// readsField answers where body selects the named field, or an invalid
// position when it does not.
func readsField(body *ast.BlockStmt, field string) token.Pos {
	var at token.Pos
	ast.Inspect(body, func(n ast.Node) bool {
		sel, ok := n.(*ast.SelectorExpr)
		if ok && sel.Sel != nil && sel.Sel.Name == field {
			at = sel.Sel.Pos()
		}
		return !at.IsValid()
	})
	return at
}
