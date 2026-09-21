// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every declared stage-automation safety defect can actually stop a rule.
//
// SuspendForSafetyDefect admits only defects its message map carries, and
// refuses anything else — which is right, because the reason is what an
// operator reads when deciding whether it is safe to resume, and a caller who
// can pass any string can turn a rule off for a reason nobody agreed was a
// safety matter.
//
// The cost of that closed door is a silent failure in the OTHER direction: a
// SafetyDefect constant somebody declares and forgets to give a message is one
// the product REFUSES to suspend on. The call site gets an error where it
// expected a safety stop, the rule keeps applying moves automatically, and
// nothing in the tree says so. That is the census this test is: the constants
// are the subject, the map is the claim, and they must agree exactly.
//
// Derived from the source rather than listing the defects here, because a gate
// that hard-codes part of its subject has become a second copy of it — and
// this one's whole job is to notice a constant nobody thought about.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"sort"
	"testing"
)

// safetyDefectsFile is where both the constants and the map live. One file by
// construction: the type, its members and their meanings are one declaration,
// and splitting them is the drift this test exists to catch.
const safetyDefectsFile = "internal/modules/deals/stageprogressionsuspend.go"

func TestEverySafetyDefectCanActuallySuspendARule(t *testing.T) {
	t.Parallel()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, safetyDefectsFile, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("parsing %s: %v", safetyDefectsFile, err)
	}

	declared := declaredSafetyDefects(file)
	explained := explainedSafetyDefects(file)

	if len(declared) == 0 {
		// A census that can fail short has already failed. If the AST walk
		// stops matching — the type renamed, the constants moved — it would
		// find nothing, compare nothing, and report PASS over a file it no
		// longer reads.
		t.Fatalf("found no SafetyDefect constants in %s: this census is reading "+
			"the wrong thing and would pass over any drift", safetyDefectsFile)
	}
	if len(explained) == 0 {
		t.Fatalf("found no safetyDefects entries in %s: same problem, from the "+
			"other side", safetyDefectsFile)
	}

	for _, name := range declared {
		if !explained[name] {
			t.Errorf("SafetyDefect %s is declared but carries no message, so "+
				"SuspendForSafetyDefect REFUSES it: a real defect of this kind "+
				"leaves the rule applying moves automatically and the caller "+
				"gets an error where it expected a safety stop", name)
		}
	}
	for name := range explained {
		if !contains(declared, name) {
			t.Errorf("safetyDefects carries a message for %s, which is not a "+
				"declared SafetyDefect — either the constant was renamed and this "+
				"entry is now dead, or a defect is admitted that nothing names", name)
		}
	}
}

// declaredSafetyDefects reads the constant names of type SafetyDefect.
func declaredSafetyDefects(file *ast.File) []string {
	var out []string
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			ident, ok := value.Type.(*ast.Ident)
			if !ok || ident.Name != "SafetyDefect" {
				continue
			}
			for _, name := range value.Names {
				out = append(out, name.Name)
			}
		}
	}
	sort.Strings(out)
	return out
}

// explainedSafetyDefects reads the keys of the safetyDefects map literal.
func explainedSafetyDefects(file *ast.File) map[string]bool {
	out := map[string]bool{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.VAR {
			continue
		}
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok || len(value.Names) != 1 || value.Names[0].Name != "safetyDefects" {
				continue
			}
			for _, v := range value.Values {
				lit, ok := v.(*ast.CompositeLit)
				if !ok {
					continue
				}
				for _, elt := range lit.Elts {
					kv, ok := elt.(*ast.KeyValueExpr)
					if !ok {
						continue
					}
					if key, ok := kv.Key.(*ast.Ident); ok {
						out[key.Name] = true
					}
				}
			}
		}
	}
	return out
}
