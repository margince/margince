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

// The provenance filter fails by being DROPPED, not by being spelled twice.
// Every list surface assembles its WHERE through a listFilters value, and one
// that omits CapturedByKind hands back an unfiltered page to a caller who did
// ask to filter — the same confident-wrong-answer capturedByKindClause refuses
// an empty enum value to avoid, arriving one layer up where nothing looks at
// the value at all.
//
// Silent at every layer: the request validates, the query runs, the page is
// well-formed, and the only symptom is rows the caller asked not to see. The
// review lists are exactly where that is hardest to notice, because a caller
// filtering for agent-created rows cannot tell "no AI wrote here" from "the
// filter was ignored".
//
// The surfaces are found rather than listed. A list surface added tomorrow is
// judged by this without anyone remembering to add it here, which is the whole
// difference between a gate and a checklist.

// provenanceField is the filter this holds: the field a listFilters value must
// carry from its caller.
const provenanceField = "CapturedByKind"

// filtersType is the struct every list surface assembles its WHERE through.
const filtersType = "listFilters"

// provenanceOwner is the one spelling of the clause the field turns into.
const provenanceOwner = "capturedByKindClause"

func TestEveryListFiltersLiteralCarriesTheProvenanceFilter(t *testing.T) {
	builds := listFiltersBuilds(t)
	if len(builds) == 0 {
		t.Fatalf("no %s is built in this module, so this gate judged nothing — either the "+
			"struct was renamed or the list surfaces have moved", filtersType)
	}
	for _, build := range builds {
		value, set := build.sets[provenanceField]
		if !set {
			t.Errorf("%s builds a %s without %s.\n\nThe surface then answers a request that "+
				"asked to filter on provenance with a page that did not, and nothing anywhere "+
				"refuses: set it from the caller's input, or give the field the zero value "+
				"explicitly beside a reason this surface has no provenance to filter on.",
				build.where, filtersType, provenanceField)
			continue
		}
		// Presence is not the claim. `CapturedByKind: nil` satisfies a gate
		// that asks only whether the key is there, and it is the exact defect
		// spelled out loud — the filter dropped, with the field named. What
		// makes an explicit zero honest is the reason beside it, which the
		// failure above already demands and nothing used to check.
		if isNilExpr(value) && !build.reasoned[provenanceField] {
			t.Errorf("%s sets %s to nil with no reason beside it.\n\nAn explicit zero is how a "+
				"surface says it has no provenance to filter on, and it is indistinguishable "+
				"from the filter being dropped unless the reason is written down. Say why this "+
				"surface has none, or set it from the caller's input.",
				build.where, provenanceField)
		}
	}
}

// The other half of the claim: one spelling means the field is read in one
// place. A second place turning CapturedByKind into SQL is how the person list
// and the lead list come to disagree about which prefix counts as an AI — the
// two are read side by side, so the disagreement reaches a person before it
// reaches a test.
//
// The subject is the FIELD, not the SQL. This module hand-writes
// `captured_by LIKE` for three other questions — was this human-written, did a
// connector capture it, has an AI written INTO it — and those are fixed
// questions rather than the caller's filter. A census over the SQL idiom would
// refuse all three; a census over who reads the field refuses only a second
// answer to the same question.
func TestTheProvenanceClauseIsBuiltInOnePlace(t *testing.T) {
	var offenders []string
	owners := 0
	for _, parsed := range moduleFiles(t) {
		carried := carriedFieldValues(parsed.file, provenanceField)
		for _, decl := range parsed.file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Body == nil || fn.Name == nil {
				continue
			}
			if fn.Name.Name == provenanceOwner {
				owners++
				continue
			}
			for _, pos := range consumingReads(fn.Body, provenanceField, provenanceOwner, carried) {
				offenders = append(offenders,
					fn.Name.Name+" ("+parsed.fset.Position(pos).String()+")")
			}
		}
	}
	if owners == 0 {
		t.Fatalf("this module declares no %s, so the provenance filter has no owner and every "+
			"list silently decides for itself", provenanceOwner)
	}
	if len(offenders) == 0 {
		return
	}
	sort.Strings(offenders)
	t.Errorf("%s is consumed outside %s by %s.\n\nOne reader is what makes it one spelling: "+
		"every list reaches the prefix rule through %s, so a second reader is a surface that "+
		"has started deciding for itself which prefix counts as an AI. Pass the field to %s "+
		"rather than reading it here.",
		provenanceField, provenanceOwner, strings.Join(offenders, ", "), provenanceOwner,
		provenanceOwner)
}

// carriedFieldValues collects the reads that merely CARRY the field from one
// place to the next: the value in `CapturedByKind: in.CapturedByKind`, and the
// right-hand side of an assignment onto a field of the same name.
//
// Carrying is not deciding. Every list surface plumbs the caller's input into
// the filter set, and a gate counting those as readers would name seven correct
// surfaces and no defect — which is how a gate stops being read.
//
// The read may sit UNDER a conversion — `CapturedByKind: capturedByKindArg(
// params.CapturedByKind)` turns a contract enum into the pointer the field
// holds — so the whole value expression is walked rather than compared. A
// converted carry is still a carry; what it is not is a second answer to which
// prefix counts.
func carriedFieldValues(file *ast.File, field string) map[ast.Node]bool {
	carried := map[ast.Node]bool{}
	carry := func(value ast.Expr) {
		ast.Inspect(value, func(node ast.Node) bool {
			if sel, isSel := node.(*ast.SelectorExpr); isSel && sel.Sel != nil && sel.Sel.Name == field {
				carried[ast.Node(sel)] = true
			}
			return true
		})
	}
	ast.Inspect(file, func(node ast.Node) bool {
		switch stmt := node.(type) {
		case *ast.KeyValueExpr:
			if key, isIdent := stmt.Key.(*ast.Ident); isIdent && key.Name == field {
				carry(stmt.Value)
			}
		case *ast.AssignStmt:
			for i, lhs := range stmt.Lhs {
				sel, isSel := lhs.(*ast.SelectorExpr)
				if !isSel || sel.Sel == nil || sel.Sel.Name != field || i >= len(stmt.Rhs) {
					continue
				}
				carry(stmt.Rhs[i])
			}
		}
		return true
	})
	return carried
}

// consumingReads reports where body reads the field for anything other than
// carrying it or handing it to its owner — the only shape that can be a second
// answer to which prefix counts.
func consumingReads(body *ast.BlockStmt, field, owner string, carried map[ast.Node]bool) []token.Pos {
	handed := map[ast.Node]bool{}
	ast.Inspect(body, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if !isCall || !callsNamed(call, owner) {
			return true
		}
		for _, arg := range call.Args {
			handed[arg] = true
		}
		return true
	})
	var found []token.Pos
	ast.Inspect(body, func(node ast.Node) bool {
		sel, isSel := node.(*ast.SelectorExpr)
		if !isSel || sel.Sel == nil || sel.Sel.Name != field {
			return true
		}
		if handed[ast.Node(sel)] || carried[ast.Node(sel)] {
			return true
		}
		found = append(found, sel.Sel.Pos())
		return true
	})
	return found
}

// filtersBuild is one assembly of the shared filter set, however it is spelled.
type filtersBuild struct {
	where    string
	sets     map[string]ast.Expr
	reasoned map[string]bool
}

// listFiltersBuilds finds every assembly of the filter set: a composite
// literal, and a zero value filled in by assignment afterwards. The second is
// the same surface written differently, and a census that reads only literals
// reports a clean module while a list surface assembled field by field carries
// no provenance filter at all.
func listFiltersBuilds(t *testing.T) []filtersBuild {
	t.Helper()
	var found []filtersBuild
	for _, parsed := range moduleFiles(t) {
		ast.Inspect(parsed.file, func(node ast.Node) bool {
			lit, isLit := node.(*ast.CompositeLit)
			if !isLit || lit.Type == nil || typeText(lit.Type) != filtersType {
				return true
			}
			build := filtersBuild{
				where:    parsed.fset.Position(lit.Pos()).String(),
				sets:     map[string]ast.Expr{},
				reasoned: map[string]bool{},
			}
			for _, element := range lit.Elts {
				kv, isKV := element.(*ast.KeyValueExpr)
				if !isKV {
					continue
				}
				key, isIdent := kv.Key.(*ast.Ident)
				if !isIdent {
					continue
				}
				build.sets[key.Name] = kv.Value
				build.reasoned[key.Name] = hasReasonNear(parsed, kv.Pos())
			}
			found = append(found, build)
			return true
		})
		found = append(found, assembledFiltersIn(parsed)...)
	}
	return found
}

// assembledFiltersIn finds the zero-value spelling: `var f listFilters` or
// `f := listFilters{}`, with the fields written on afterwards.
func assembledFiltersIn(parsed moduleFile) []filtersBuild {
	var found []filtersBuild
	for _, decl := range parsed.file.Decls {
		fn, isFunc := decl.(*ast.FuncDecl)
		if !isFunc || fn.Body == nil {
			continue
		}
		zeroed := zeroValuesOf(fn.Body, filtersType)
		if len(zeroed) == 0 {
			continue
		}
		builds := map[string]*filtersBuild{}
		for name := range zeroed {
			builds[name] = &filtersBuild{
				where:    fn.Name.Name + " (" + parsed.fset.Position(fn.Pos()).String() + ")",
				sets:     map[string]ast.Expr{},
				reasoned: map[string]bool{},
			}
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			assign, isAssign := node.(*ast.AssignStmt)
			if !isAssign {
				return true
			}
			for i, lhs := range assign.Lhs {
				sel, isSel := lhs.(*ast.SelectorExpr)
				if !isSel {
					continue
				}
				base, isIdent := sel.X.(*ast.Ident)
				if !isIdent || builds[base.Name] == nil {
					continue
				}
				build := builds[base.Name]
				if i < len(assign.Rhs) {
					build.sets[sel.Sel.Name] = assign.Rhs[i]
				}
				build.reasoned[sel.Sel.Name] = hasReasonNear(parsed, assign.Pos())
			}
			return true
		})
		for _, name := range sortedKeys(builds) {
			found = append(found, *builds[name])
		}
	}
	return found
}

func sortedKeys(builds map[string]*filtersBuild) []string {
	names := make([]string, 0, len(builds))
	for name := range builds {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// isNilExpr reports the untyped nil, which is what an explicitly dropped filter
// is written as.
func isNilExpr(expr ast.Expr) bool {
	ident, isIdent := expr.(*ast.Ident)
	return isIdent && ident.Name == "nil"
}

// hasReasonNear reports whether a comment sits on the line above pos or at the
// end of its own line — the two places a reason is actually written.
func hasReasonNear(parsed moduleFile, pos token.Pos) bool {
	at := parsed.fset.Position(pos).Line
	for _, group := range parsed.file.Comments {
		for _, comment := range group.List {
			line := parsed.fset.Position(comment.Pos()).Line
			if line == at || line == at-1 {
				return true
			}
		}
	}
	return false
}
