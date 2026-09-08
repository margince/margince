// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// "Which activities belong to this account" has ONE answer, spelled twice.
//
// It has to be spelled twice: the timeline list, the account view and the
// roll-up ask it from `activities`, the context walk asks it from `search`, and
// a module never imports a sibling (ADR-0054). The tree already accepts that
// trade for the project-scope predicate, with a comment on each half saying
// "change one, change both".
//
// A comment is not a mechanism. This is: the two spellings of the ARMS — the
// account an activity is filed against, the account its deal belongs to, and
// the employer of the contact it is about — must be the same text. An arm that
// gains a condition on one side and not the other is what makes an account's
// timeline and its context walk disagree about which meetings it had, and the
// disagreement is silent on both sides.
//
// The arms rather than the shapes, deliberately: the two shapes differ for a
// reason that will not go away (a predicate takes the account as a bind, a
// producer holds the activity and needs the accounts), while the arms are the
// answer itself.

import (
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"strings"
	"testing"
)

// accountReachFiles are the two files that spell the walk.
var accountReachFiles = []string{
	"internal/modules/activities/companyscope.go",
	"internal/modules/search/graphorgreach.go",
}

// accountReachArms are the constants each of those files must spell, and spell
// identically. EVERY constant, not just the first: the arms are split across
// two of them because one is shared with a producer that stops short of the
// last arm, and holding only the shared half equal would leave the arm that
// split them free to drift — which is this gate's whole subject.
var accountReachArms = []string{"companyArms", "participantEmployerArm"}

func TestTheAccountReachWalkIsOneAnswer(t *testing.T) {
	t.Parallel()
	for _, arm := range accountReachArms {
		texts := map[string]string{}
		for _, file := range accountReachFiles {
			text, found := constText(t, file, arm)
			if !found {
				t.Fatalf("%s no longer declares %s — the account-reach walk was renamed or moved, and "+
					"this gate stopped comparing anything rather than failing", file, arm)
			}
			texts[file] = text
		}
		first := accountReachFiles[0]
		for _, other := range accountReachFiles[1:] {
			if normalizeSQL(texts[first]) != normalizeSQL(texts[other]) {
				t.Errorf("the %s arm differs between %s and %s — one of them has gained or "+
					"lost a condition, so the account's timeline and its context walk no longer agree "+
					"about which activities belong to it:\n  %s: %s\n  %s: %s",
					arm, first, other,
					first, normalizeSQL(texts[first]),
					other, normalizeSQL(texts[other]))
			}
		}
		// Both halves must actually SAY something. Two empty strings compare
		// equal, and a gate that passes over nothing is the failure mode this
		// whole file is about.
		for file, text := range texts {
			if strings.TrimSpace(text) == "" {
				t.Errorf("%s spells %s as an empty string, so the comparison above proved nothing", file, arm)
			}
		}
	}
}

// constText answers the string literal a named package-level constant holds.
func constText(t *testing.T, file, name string) (string, bool) {
	t.Helper()
	fset := token.NewFileSet()
	parsed, err := parser.ParseFile(fset, file, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", file, err)
	}
	for _, decl := range parsed.Decls {
		gen, isGen := decl.(*ast.GenDecl)
		// VAR as well as CONST: an arm that calls the shared employment
		// predicate cannot be a constant, and reading only constants would
		// leave this gate comparing nothing on the day the arms adopted it.
		if !isGen || (gen.Tok != token.CONST && gen.Tok != token.VAR) {
			continue
		}
		for _, spec := range gen.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue {
				continue
			}
			// A spec whose names and values are not one-to-one is not read by
			// POSITION — a `const a, b = f()` or an iota run pairs a name with
			// an expression that is not its own, and reading it by index is how
			// a refactor quietly hands this gate the wrong text to compare.
			if len(value.Names) != len(value.Values) {
				continue
			}
			for i, ident := range value.Names {
				if ident.Name != name {
					continue
				}
				// The expression AS WRITTEN, not the string it resolves to.
				//
				// An arm that calls the shared employment predicate is a
				// concatenation rather than a literal, and a reader that folded
				// it would have to put something in the call's place — every
				// fold available renders two DIFFERENT calls as the same
				// placeholder, so the gate would stop being able to tell
				// `IsCurrentSQL("r.ended_at")` from `IsCurrentSQL("emp.ended_at")`.
				// That distinction is this gate's whole subject.
				//
				// Source text keeps it: two declarations that are identical
				// character for character are identical however they are built,
				// which is the claim being made.
				var written strings.Builder
				if err := printer.Fprint(&written, fset, value.Values[i]); err != nil {
					t.Fatalf("printing %s in %s: %v", name, file, err)
				}
				return written.String(), true
			}
		}
	}
	return "", false
}

// normalizeSQL collapses the whitespace two files indent differently, so the
// comparison is about the SQL rather than about where each copy sits on the
// page.
func normalizeSQL(text string) string {
	return strings.Join(strings.Fields(text), " ")
}
