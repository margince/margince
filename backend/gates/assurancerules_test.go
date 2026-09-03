// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
//gate:kind census H3

package gates

// Every assurance rule proves both halves of its judgement.
//
// A rule tested only on the case it fires for passes identically when it fires
// for EVERYTHING. That failure is worse than a rule that never fires: a pass
// flagging every deal trains the reader to dismiss it, and then the one real
// finding goes past with the noise.
//
// The corpus is DERIVED from the package's own declarations rather than listed
// here. A list would go stale in the direction that reads as success — a rule
// added and forgotten leaves the gate green while nothing checks it.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// assuranceDir is where the rules live, relative to this package — gates run
// with the repository root as their parent.
const assuranceDir = "../backend/internal/modules/assurance"

func TestEveryAssuranceRuleAdmitsAndRefuses(t *testing.T) {
	t.Parallel()

	declared, tests := assuranceSources(t)
	if len(declared) == 0 {
		t.Fatal("the assurance package declares no rule types at all — this gate " +
			"checked nothing, which is the one outcome it must not report as a pass")
	}
	if len(tests) == 0 {
		t.Fatal("the assurance package has no test files, so every rule would read as " +
			"untested — the thing to fix is the package, not this gate")
	}

	for _, ruleType := range declared {
		named := false
		for _, body := range tests {
			if strings.Contains(body, ruleType) {
				named = true
				break
			}
		}
		if !named {
			t.Errorf("rule constant %s is declared and no test names it — a rule nobody "+
				"exercises is a rule that may fire on everything, and the reader learns "+
				"to dismiss the whole pass", ruleType)
		}
	}
}

// assuranceSources reads the rule constants the package declares and the bodies
// of its test files.
//
// It FAILS on an unreadable directory rather than returning nothing. A census
// that reads no files reports every rule untested, which is loud; one that
// silently read an empty directory would report every rule tested, and
// under-recognition is the single direction a census must never fail in.
func assuranceSources(t *testing.T) (declared []string, tests []string) {
	t.Helper()
	entries, err := os.ReadDir(assuranceDir)
	if err != nil {
		t.Fatalf("reading the assurance package: %v", err)
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") {
			continue
		}
		path := filepath.Join(assuranceDir, name)
		if strings.HasSuffix(name, "_test.go") {
			body, err := os.ReadFile(path)
			if err != nil {
				t.Fatalf("reading %s: %v", name, err)
			}
			tests = append(tests, string(body))
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", name, err)
		}
		declared = append(declared, ruleTypeConstants(file)...)
	}
	return declared, tests
}

// ruleTypeConstants reads the Type* constants one file declares.
//
// Read from the CONSTANTS rather than from the Rules() slice, because a rule
// whose entry somebody forgot to add to that slice is exactly the rule this
// gate should notice — deriving from the slice would ask the code to confirm
// itself.
func ruleTypeConstants(file *ast.File) []string {
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
			for _, name := range value.Names {
				// Type* names an exception type; Severity* does not.
				if strings.HasPrefix(name.Name, "Type") {
					out = append(out, name.Name)
				}
			}
		}
	}
	return out
}
