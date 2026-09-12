// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H3

package gates

// Every way a contact can arrive has a decided disclosure duty.
//
// The acquisition vocabulary lives in contacts; the duty it incurs is decided in
// consent. A module may not import a sibling, so the two halves cannot check
// each other in code — which is exactly the shape gates/ exists for.
//
// Without this, a tenth acquisition kind added in contacts reaches DutyFor's
// default arm and is silently treated as Art. 14. That default is deliberate
// and correct as a fail-safe, but it must not become the way new kinds get
// decided: an unconsidered kind should stop somebody and ask, not be absorbed.

import (
	"go/ast"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// acquisitionKindConst matches the constant NAMES contacts declares, so the
// corpus is derived from the declaration rather than from a copy of its values.
var acquisitionKindConst = regexp.MustCompile(`^Acquired[A-Z]`)

// TestEveryAcquisitionKindReachesADecidedDuty derives the vocabulary from
// contacts/acquisition.go and proves consent's noticerule.go names each one.
//
// It matches the STRING VALUES against the switch arms' own literals rather
// than calling DutyFor — gates may not import a module. A kind whose literal
// appears in no arm of the switch falls to the default, which is what this
// fails on.
func TestEveryAcquisitionKindReachesADecidedDuty(t *testing.T) {
	t.Parallel()

	kinds := acquisitionKinds(t)
	// Under-recognition is the one way this must not fail: a parse that found
	// nothing would loop zero times and report PASS over an undecided
	// vocabulary.
	if len(kinds) < 9 {
		t.Fatalf("read %d acquisition kinds from contacts/acquisition.go, want at least 9: "+
			"the census has stopped seeing its subject", len(kinds))
	}

	decided := decidedKinds(t)
	if len(decided) < 8 {
		t.Fatalf("read %d decided kinds from consent/noticerule.go, want at least 8: "+
			"the census has stopped seeing the decision it checks", len(decided))
	}

	for _, kind := range kinds {
		if !decided[kind] {
			t.Errorf("acquisition kind %q reaches no arm of DutyFor in "+
				"internal/modules/consent/noticerule.go, so it falls to the default and is "+
				"treated as an Art. 14 duty by accident. Decide it: data the SUBJECT handed us "+
				"owes no case (Art. 13 was discharged at collection); data from anywhere else "+
				"owes an Art. 14 notice within a month", kind)
		}
	}
}

// acquisitionKinds reads the string value of every Acquired* constant.
func acquisitionKinds(t *testing.T) []string {
	t.Helper()
	file := parseModuleFile(t, filepath.Join(moduleRoot(t), "internal", "modules", "contacts", "acquisition.go"))
	var out []string
	for _, decl := range file.Decls {
		gen, isGen := decl.(*ast.GenDecl)
		if !isGen || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue || len(value.Names) == 0 || len(value.Values) == 0 {
				continue
			}
			if !acquisitionKindConst.MatchString(value.Names[0].Name) {
				continue
			}
			text, isText := literalString(value.Values[0])
			if isText {
				out = append(out, text)
			}
		}
	}
	return out
}

// decidedKinds reads every string literal appearing in a case clause of
// noticerule.go — the arms of the switch that decides a duty.
func decidedKinds(t *testing.T) map[string]bool {
	t.Helper()
	file := parseModuleFile(t, filepath.Join(moduleRoot(t), "internal", "modules", "consent", "noticerule.go"))
	out := map[string]bool{}
	// DutyFor's own body, not the whole file: a second switch added here for an
	// unrelated reason must not be able to satisfy this census with a literal
	// that never reaches the decision.
	for _, decl := range file.Decls {
		fn, isFn := decl.(*ast.FuncDecl)
		if !isFn || fn.Name.Name != "DutyFor" {
			continue
		}
		ast.Inspect(fn.Body, func(node ast.Node) bool {
			clause, isClause := node.(*ast.CaseClause)
			if !isClause {
				return true
			}
			for _, expr := range clause.List {
				if text, isText := literalString(expr); isText {
					out[text] = true
				}
			}
			return true
		})
	}
	return out
}

// parseModuleFile reads a module source file for this census. It reads the
// bytes and hands them to the package's own parseGateFile rather than parsing
// a second way, so both censuses see a file identically.
func parseModuleFile(t *testing.T, path string) *ast.File {
	t.Helper()
	source, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return parseGateFile(t, path, source)
}

// literalString returns the text of a quoted string literal, decoded.
//
// gatekit.LiteralText rather than lit.Value with the quotes trimmed: Value is
// the SOURCE text, so a literal written with an escape reaches a trimming
// reader as its backslashes and matches nothing — a census that then reports a
// clean tree over the shape it exists to find.
func literalString(expr ast.Expr) (string, bool) {
	return gatekit.LiteralText(expr)
}
