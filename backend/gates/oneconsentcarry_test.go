// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// The consent carry — what happens to a retiring record's consent when another
// record survives it — is spelled once inside the people module.
//
// It was spelled three times: a person merge, a lead merge, and a lead's
// promotion to a person, each with its own copy of one CTE differing only in a
// key column and a literal. Consent is the domain where a fix applied to two
// of three copies is a lawful-processing defect rather than an untidiness: the
// rule the CTE encodes is "a withdrawal always wins", and a copy that missed a
// correction turns an opt-out back into a grant.
//
// The three had already drifted, on whether the proof events follow the state,
// and the difference lived only in three prose comments in three files. It is
// declared in the spec now, and asserted against real rows by
// TestEachConsentCarryProvesItsProofRule.
//
// SCOPED TO people, deliberately. The consent module owns these tables and
// writes them for its own reasons — a preference centre save, a double
// opt-in confirmation — and those are not carries. What this gate governs is
// the SIBLING that reaches across into them under the package's sanctioned
// cross-aggregate ownership, which is where a second copy went unnoticed
// three times.
//
// WHAT IT CANNOT SEE: a carry assembled from SQL fragments that never spell
// the statement in one literal. Every write in this tree spells it whole.
//
// The file count is not enough on its own, and neither is the waiver below.
// A second carry added BESIDE the first in the same file keeps the count at
// one, and the consent module's waiver is keyed by file while the thing it
// ratifies is a set of statements. Two further tests close both: every
// consent-writing function in the owning file has to be reachable from
// carryConsent, and the RE-KEYING statement — the one that moves a consent row
// from one subject to another, which is what makes a write a carry rather than
// a record of a decision — may appear in no other file at all, waived or not.

import (
	"go/ast"
	"go/token"
	"os"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// consentStateStatement matches the writes a carry makes: the withdrawal flip,
// the proof row it appends, and the re-point that moves the state onto the
// survivor. A read of either table is not a subject — the defect is a second
// implementation of the RULE, not a second reader of the rows.
var consentStateStatement = regexp.MustCompile(
	`(?is)INSERT\s+INTO\s+consent_event\b|UPDATE\s+person_consent\b|DELETE\s+FROM\s+person_consent\b`)

// writesConsentState reports whether a file carries any of those statements.
func writesConsentState(_ string, file *ast.File) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if !ok || lit.Kind != token.STRING || !consentStateStatement.MatchString(gatekit.TextOf(lit)) {
			return !found
		}
		found = true
		return false
	})
	return found
}

// theCarryFile is where the carry lives. Named because the two tests below ask
// questions ABOUT that file rather than about a count.
//
// Held by: TestTheConsentCarryIsSpelledOnceInPeople (backend/gates/oneconsentcarry_test.go) — which fails
// if the people module's consent writes are in any other file, and fails again
// if they have moved out of this one.
const theCarryFile = "internal/modules/people/consentcarry.go"

func TestTheConsentCarryIsSpelledOnceInPeople(t *testing.T) {
	t.Parallel()
	scope := gatekit.Scope{
		Roots:   []string{"internal/modules/people"},
		Subject: writesConsentState,
		Exempt: gatekit.Waive(map[string]string{
			"internal/modules/consent/store.go":        "the consent module OWNS these tables and writes them for its own reasons — a preference-centre save, a double opt-in confirmation, a withdrawal a person asked for. None of those is a carry: they record what a subject decided, where a carry decides what happens to a decision when the record holding it retires. The two would not share an implementation even if they were in one package",
			"internal/modules/consent/consentproof.go": "the paired state-and-proof write the consent module's own Record makes, split out of store.go for the file-length cap. Same reason as store.go beside it: recording a decision a subject made, never deciding what becomes of one when a record retires",
		}),
	}
	inside := scope.Files(t)
	if len(inside) > 1 {
		var where []string
		for _, f := range inside {
			where = append(where, f.Path)
		}
		t.Errorf("the people module writes consent state from %d files:\n\t%s\n\n"+
			"One carry, with its differences declared in the spec rather than left to a reader comparing "+
			"files. A withdrawal that wins in two copies and not the third is a lawful-processing defect",
			len(inside), strings.Join(where, "\n\t"))
	}
	if len(inside) == 1 && inside[0].Path != theCarryFile {
		t.Errorf("the people module's consent writes have moved to %s; the tests below ask their questions "+
			"of %s and would be reading an empty file", inside[0].Path, theCarryFile)
	}
}

// Every function that writes consent state in the owning file is part of the
// one carry — reachable from carryConsent rather than merely next to it.
//
// A file count cannot see the copy this catches. `carryImportedConsent` added
// beside `carryConsent`, with its own flip and its own re-point, leaves the
// count at one and is a second answer to "what happens to a withdrawal when the
// record holding it retires".
func TestEveryConsentWriteInTheCarryFileBelongsToTheCarry(t *testing.T) {
	t.Parallel()
	source, err := os.ReadFile(theCarryFile)
	if err != nil {
		t.Fatalf("reading the carry: %v", err)
	}
	file := parseGateFile(t, theCarryFile, source)
	reachable := calledFrom(file, "carryConsent")
	reachable["carryConsent"] = true
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil || !holdsAConsentStatement(fn.Body) {
			continue
		}
		if !reachable[fn.Name.Name] {
			t.Errorf("%s writes consent state but nothing in carryConsent reaches it, so it is a second "+
				"answer to what happens to a decision when its record retires. Route it through the spec "+
				"table, or say in consentCarries why it is a carry of its own", fn.Name.Name)
		}
	}
}

// reKeyedConsent matches the statement that MOVES a consent row from one
// subject to another: an UPDATE on either table whose SET target is a format
// verb rather than a column name.
//
// The verb is the tell, and it is a better one than the column names would be.
// A carry does not know which subject key it is writing — that is what the spec
// table decides — so it parameterises the identifier; every other write of
// these tables names the column it means. Recording a decision never re-keys
// the row the decision sits on, which is why this can tell a preference-centre
// save from a carry where the file-keyed waiver above cannot.
var reKeyedConsent = regexp.MustCompile(`(?is)UPDATE\s+(person_consent|consent_event)\b[^;]*?\bSET\s+%\[\d+\]s\s*=`)

// TestOnlyTheCarryReKeysAConsentRow sweeps EVERY module, waiver or not, because
// the waiver above ratifies a file and this obligation is about statements.
//
// The root is the whole module tier rather than the carry's own file. Rooting
// it at the file would have made the sweep tautological: gatekit reports what
// lies outside the roots, so a second re-key added anywhere else would be the
// only thing it could see — and a re-key added to the consent module, which the
// scope above waives wholesale, is exactly the copy this is here to catch.
func TestOnlyTheCarryReKeysAConsentRow(t *testing.T) {
	t.Parallel()
	scope := gatekit.Scope{
		Roots:   []string{"internal/modules"},
		Subject: func(_ string, file *ast.File) bool { return holdsMatchingLiteral(file, reKeyedConsent) },
		Exempt:  gatekit.Waive(map[string]string{}),
	}
	inside := scope.Files(t)
	var where []string
	for _, f := range inside {
		where = append(where, f.Path)
	}
	if len(inside) != 1 || where[0] != theCarryFile {
		t.Errorf("consent rows are re-keyed in %d file(s):\n\t%s\n\nExactly one, and it must be %s. "+
			"Re-keying a consent row is what makes a write a carry rather than a record of a decision, so "+
			"a second one is a second answer to what happens to a withdrawal when its record retires — and "+
			"none at all means the carry has moved and this gate is reading an empty tree",
			len(inside), strings.Join(where, "\n\t"), theCarryFile)
	}
}

// calledFrom returns the names of the functions called, at any depth, from the
// named one — a one-package call graph, which is all this file needs.
func calledFrom(file *ast.File, root string) map[string]bool {
	bodies := map[string]*ast.BlockStmt{}
	for _, decl := range file.Decls {
		if fn, ok := decl.(*ast.FuncDecl); ok && fn.Body != nil {
			bodies[fn.Name.Name] = fn.Body
		}
	}
	reached := map[string]bool{}
	var walk func(name string)
	walk = func(name string) {
		body, ok := bodies[name]
		if !ok {
			return
		}
		ast.Inspect(body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			if callee, ok := call.Fun.(*ast.Ident); ok && !reached[callee.Name] {
				reached[callee.Name] = true
				walk(callee.Name)
			}
			return true
		})
	}
	walk(root)
	return reached
}

// holdsAConsentStatement is writesConsentState asked of one function body.
func holdsAConsentStatement(body *ast.BlockStmt) bool {
	found := false
	ast.Inspect(body, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if ok && lit.Kind == token.STRING && consentStateStatement.MatchString(gatekit.TextOf(lit)) {
			found = true
		}
		return !found
	})
	return found
}

// holdsMatchingLiteral reports whether any string literal in the file matches.
func holdsMatchingLiteral(file *ast.File, pattern *regexp.Regexp) bool {
	found := false
	ast.Inspect(file, func(n ast.Node) bool {
		lit, ok := n.(*ast.BasicLit)
		if ok && lit.Kind == token.STRING && pattern.MatchString(gatekit.TextOf(lit)) {
			found = true
		}
		return !found
	})
	return found
}
