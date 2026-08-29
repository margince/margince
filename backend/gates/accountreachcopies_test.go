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
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// accountReachArms names the two files that spell the arms, and the constant
// each spells them into.
var accountReachArms = []struct {
	file string
	// name is the constant each file spells the arms into.
	name string
}{
	{file: "internal/modules/activities/orgscope.go", name: "orgArms"},
	{file: "internal/modules/search/graphorgreach.go", name: "orgArms"},
}

func TestTheAccountReachWalkIsOneAnswer(t *testing.T) {
	t.Parallel()
	texts := map[string]string{}
	for _, spelling := range accountReachArms {
		text, found := constText(t, spelling.file, spelling.name)
		if !found {
			t.Fatalf("%s no longer declares %s — the account-reach walk was renamed or moved, and "+
				"this gate stopped comparing anything rather than failing",
				spelling.file, spelling.name)
		}
		texts[spelling.file] = text
	}
	first := accountReachArms[0]
	for _, other := range accountReachArms[1:] {
		if normalizeSQL(texts[first.file]) != normalizeSQL(texts[other.file]) {
			t.Errorf("the account-reach arms differ between %s and %s — one of them has gained or "+
				"lost a condition, so the account's timeline and its context walk no longer agree "+
				"about which activities belong to it:\n  %s: %s\n  %s: %s",
				first.file, other.file,
				first.file, normalizeSQL(texts[first.file]),
				other.file, normalizeSQL(texts[other.file]))
		}
	}
	// Both halves must actually SAY something. Two empty strings compare equal,
	// and a gate that passes over nothing is the failure mode this whole file
	// is about.
	for file, text := range texts {
		if strings.TrimSpace(text) == "" {
			t.Errorf("%s spells the arms as an empty string, so the comparison above proved nothing", file)
		}
	}
}

// constText answers the string literal a named package-level constant holds.
func constText(t *testing.T, file, name string) (string, bool) {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", file, err)
	}
	for _, decl := range parsed.Decls {
		gen, isGen := decl.(*ast.GenDecl)
		if !isGen || gen.Tok != token.CONST {
			continue
		}
		for _, spec := range gen.Specs {
			value, isValue := spec.(*ast.ValueSpec)
			if !isValue {
				continue
			}
			for i, ident := range value.Names {
				if ident.Name != name || i >= len(value.Values) {
					continue
				}
				lit, isLit := value.Values[i].(*ast.BasicLit)
				if !isLit || lit.Kind != token.STRING {
					return "", false
				}
				text, err := strconv.Unquote(lit.Value)
				if err != nil {
					return "", false
				}
				return text, true
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
