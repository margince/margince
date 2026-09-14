// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every contact-creation door decides, out loud, when the acquisition happened.
//
// contact_acquisition_evidence.occurred_at is what the Art. 14 disclosure
// deadline runs from: compose/noticecaseopen.go dates the duty from
// `coalesce(occurred_at, captured_at)`. A door that leaves it empty gives the
// contact a deadline starting the day the row was written, which for anything
// captured in arrears is late — and invisibly so, because the case then shows a
// deadline a month away rather than one already missed.
//
// THE CENSUS IS OVER createContact CALLERS, not over a list of filenames. An
// earlier version of this gate named two files, and it would have passed while a
// third door wrote an undated acquisition and a fourth omitted the field
// entirely. A door added tomorrow has to appear here and be decided about,
// which is the only shape that survives somebody who has not read this comment.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

// acquisitionDecisions records, per contact-creation door, what it does about
// the acquisition's TIME. The census below derives the doors from the call
// graph and compares them against this map in both directions, so a new door
// fails until somebody writes its line and a stale line fails once its function
// is gone. The default is not "undated", it is "decide and say why".
//
// The key is `file.go:function`, which is what the failure message prints.
//
// gatekit:fixture the contact-creation doors and how each dates its acquisition
var acquisitionDecisions = map[string]string{
	"ensure.go:ensureContact": "dates it from the message, through acquisitionFromCapture",
	"ensurechannel.go:offerChannelContact": "dates it from the message, through " +
		"acquisitionFromCapture",
	"contact.go:createContactInTx": "forwards whatever the caller declared, which is right for " +
		"a typed create: the API door is the one place a human can state an acquisition, and " +
		"an unset one records unknown_legacy",
	"promote.go:promoteTarget": "states NO time, deliberately. A lead becoming a contact is not " +
		"a fresh acquisition — the act happened when the lead arrived, which this module " +
		"cannot see from here — so it records unknown_legacy rather than dating the duty from " +
		"the promotion",
}

// TestEveryContactCreationDoorDecidesWhenItAcquiredSomebody is the census.
func TestEveryContactCreationDoorDecidesWhenItAcquiredSomebody(t *testing.T) {
	t.Parallel()
	found := createContactCallers(t)
	var undeclared []string
	for _, caller := range found {
		if _, declared := acquisitionDecisions[caller]; !declared {
			undeclared = append(undeclared, caller)
		}
	}
	if len(undeclared) > 0 {
		sort.Strings(undeclared)
		t.Errorf("%d contact-creation door(s) are not in acquisitionDecisions:\n\t%s\n\n"+
			"Every door decides when the acquisition happened, because the Art. 14 deadline "+
			"runs from occurred_at and falls back to the row's own write. Add a line saying "+
			"what this one does and why — dating it from something it knows, or stating that "+
			"it genuinely cannot say.",
			len(undeclared), strings.Join(undeclared, "\n\t"))
	}
	// The other direction: a line for a door that no longer exists is a
	// decision nobody is holding to, and it hides the next door's absence by
	// making the count look right.
	var stale []string
	present := map[string]bool{}
	for _, caller := range found {
		present[caller] = true
	}
	for declared := range acquisitionDecisions {
		if !present[declared] {
			stale = append(stale, declared)
		}
	}
	if len(stale) > 0 {
		sort.Strings(stale)
		t.Errorf("%d declared door(s) no longer call createContact:\n\t%s\n\n"+
			"Drop the line, or point it at what replaced the function.",
			len(stale), strings.Join(stale, "\n\t"))
	}
}

// TestBothCaptureDoorsDateTheAcquisitionFromTheMessage holds the claim in
// acquiredwhen.go's acquisitionFromCapture doc comment: both capture doors go
// through that one builder, so neither can start dating an acquisition from
// its own write while the other reads the captured history.
//
// Only one of them obviously runs behind the message — the mail sink's delay is
// a post-commit step, while the verdict path's can be days — so a later author
// could reasonably think the other needs no dating.
func TestBothCaptureDoorsDateTheAcquisitionFromTheMessage(t *testing.T) {
	t.Parallel()
	dir := filepath.Join(repoRoot, "backend", "internal", "modules", "contacts")
	for _, name := range []string{"ensure.go", "ensurechannel.go"} {
		src, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil {
			t.Fatalf("%s no longer exists — this gate names a door that is gone, point it "+
				"at what replaced the file: %v", name, err)
		}
		if !strings.Contains(string(src), "acquisitionFromCapture(") {
			t.Errorf("%s creates a contact from a captured message without calling "+
				"acquisitionFromCapture. The acquisition then has no occurred_at, and the "+
				"Art. 14 deadline runs from the row's own write rather than from the "+
				"message — which is late for anything captured in arrears, and looks on time.",
				name)
		}
	}
}

// createContactCallers answers every `file.go:function` in the contacts package
// whose body calls createContact.
//
// Scoped to that package because createContact is unexported, so nothing
// outside it can be a door. A door reached through a wrapper still shows up as
// the wrapper, which is the honest unit: the wrapper is what has to decide.
func createContactCallers(t *testing.T) []string {
	t.Helper()
	dir := filepath.Join(repoRoot, "backend", "internal", "modules", "contacts")
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("reading the contacts package: %v", err)
	}
	fset := token.NewFileSet()
	var out []string
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") || strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(dir, e.Name()), nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", e.Name(), err)
		}
		for _, decl := range file.Decls {
			fn, isFunc := decl.(*ast.FuncDecl)
			if !isFunc || fn.Body == nil {
				continue
			}
			var calls bool
			ast.Inspect(fn.Body, func(n ast.Node) bool {
				call, isCall := n.(*ast.CallExpr)
				if !isCall {
					return true
				}
				if ident, isIdent := call.Fun.(*ast.Ident); isIdent && ident.Name == "createContact" {
					calls = true
					return false
				}
				return true
			})
			if calls {
				out = append(out, e.Name()+":"+fn.Name.Name)
			}
		}
	}
	if len(out) == 0 {
		t.Fatal("no createContact callers found — this gate is reading a shape that is gone")
	}
	return out
}
