// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every writer of `Activity.AudienceReason` is named here with the test that
// proves it withholds the reason from a reader who may not see the content.
//
// Why a captured message is held describes what it is ABOUT — `personnel`,
// `legal`, `security_incident` — so a colleague who may not read the message
// must not learn why it is held either. `crm.yaml` says it: absent whenever
// content_state is withheld.
//
// The field is optional on the wire. A reason that leaks and a reason that was
// never set produce responses no client can tell apart, and no downstream
// assertion fails on either. Both halves of that have already happened: the
// person 360 assembles its own SELECT rather than using the shared projection
// and shipped without the column at all, so the record timeline — the one
// screen where an owner decides whether to share a thread — never received it.
//
// This gate does NOT re-check the withholding by reading SQL. An earlier
// version did, matching `a.audience` in the statement text, and it passed on
// `a.audience AS audience` and on any table alias that was not the letter `a`
// (`sarsections.go` uses `at`, `export_scope.go` uses `av`, so that is ordinary
// style here rather than a contrivance). A census that can fail short has
// already failed (AGENTS.md rule 8), and a text proxy for a behaviour is the
// shape that fails short.
//
// What it asserts instead is the part a text scan cannot fake: WHO writes the
// field, derived from the AST, so a third writer cannot appear unnoticed
// whatever SQL it used to read the column. The withholding itself is proved by
// the tests named below, each mutation-checked in both directions — a version
// that leaks the reason and a version that never sets it both fail.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// The writers, and the behaviour test that proves each one. A new writer joins
// this map with its own proof, or the census below fails.
//
// gatekit:fixture the name of each writer's proof, not a reason to excuse it — the
// census fails when a writer has no entry, and when an entry names a test that
// does not exist.
var audienceReasonWriters = map[string]string{
	"internal/modules/activities/activityprojection.go": "TestAWithheldRowCarriesNoAudienceReason",
	"internal/compose/person360/sectionstimeline.go":    "TestThePersonPageWithholdsALimitedMessagesReasonFromAColleague",
}

func TestEveryAudienceReasonWriterIsProvedToWithholdIt(t *testing.T) {
	t.Parallel()
	found := writersOfAudienceReason(t)

	for path := range found {
		if _, declared := audienceReasonWriters[path]; !declared {
			t.Errorf("%s assigns Activity.AudienceReason and no test in this census proves it "+
				"nils the reason when the content is withheld. A leaked reason and an absent one "+
				"are the same bytes on the wire, so nothing downstream fails on either: write the "+
				"test, mutation-check it in both directions, and name it here.", path)
		}
	}
	for path, proof := range audienceReasonWriters {
		if !found[path] {
			t.Errorf("%s is named here as a writer of Activity.AudienceReason but no longer "+
				"assigns it — drop the entry rather than leave the census guarding a file "+
				"that moved", path)
			continue
		}
		if !testExists(t, proof) {
			t.Errorf("%s names %s as its proof, and no test by that name exists in the tree",
				path, proof)
		}
	}
}

// writersOfAudienceReason walks the tree for an assignment to the contract
// field. Derived rather than listed: a new 360, export or overlay that fills
// the field joins the census on the commit it is written, whatever SQL spelling
// or table alias it used to read the column.
func writersOfAudienceReason(t *testing.T) map[string]bool {
	t.Helper()
	out := map[string]bool{}
	goFilesUnderTree(t, func(path, _ string) {
		// The generated contract DECLARES the field; it does not write one.
		if strings.HasPrefix(path, "internal/contracts/") {
			return
		}
		file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			assign, ok := n.(*ast.AssignStmt)
			if !ok {
				return true
			}
			for _, lhs := range assign.Lhs {
				if sel, ok := lhs.(*ast.SelectorExpr); ok && sel.Sel.Name == "AudienceReason" {
					out[path] = true
				}
			}
			return true
		})
	})
	return out
}

// testExists asks whether a test function of that name is declared anywhere in
// the tree, so a census entry cannot go on naming a proof that was renamed or
// deleted.
func testExists(t *testing.T, name string) bool {
	t.Helper()
	needle := "func " + name + "("
	var found bool
	err := filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if found || d.IsDir() || !strings.HasSuffix(path, "_test.go") {
			return nil
		}
		src, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		if strings.Contains(string(src), needle) {
			found = true
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the tree for %s: %v", name, err)
	}
	return found
}
