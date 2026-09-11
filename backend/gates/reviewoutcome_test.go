// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind claim H2

package gates

// Every way a held message can settle closes its review.
//
// A refusal freezes a message and opens a review for it. That review has to end
// when the message's fate is decided, and until the slice this gate ships with,
// three of the four ways that happens left it live — most sharply the one that
// reads like success: a held message resumed and ALLOWED went out and left a
// decider still being asked whether it may go.
//
// activities names what happened as a ReviewOutcome; compose maps each onto the
// state and the sentence consent writes on the row. The map's own default
// refuses an outcome it does not know, so a missing case cannot send a message
// with a wrong account of why it stopped — but it CAN fail a cancel at runtime,
// in a transaction a rep is waiting on, over a constant somebody added upstairs
// and never came back to.
//
// So the map is checked against the vocabulary here, where adding a constant
// and forgetting the case is a red test rather than a production fault.
//
// WHEN YOU ADD AN OUTCOME: add its case to reviewClosureFor in
// internal/compose/reviewdecision.go. That is the whole obligation; this gate
// exists only to make forgetting it loud.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"
)

const (
	// Where the vocabulary is declared.
	reviewOutcomeFile = "internal/modules/activities/scheduledsend.go"
	// Where each member of it is translated.
	reviewClosureFile = "internal/compose/reviewdecision.go"
	// The function whose cases are the translation.
	reviewClosureFunc = "reviewClosureFor"
	// The type whose constants are the vocabulary.
	reviewOutcomeType = "ReviewOutcome"
	// The package the translation names them under.
	reviewOutcomePackage = "activities"
)

func TestEveryReviewOutcomeHasAClosure(t *testing.T) {
	t.Parallel()

	declared := declaredReviewOutcomes(t)
	if len(declared) == 0 {
		// A vocabulary that parsed to nothing would make this gate pass by
		// reading no file at all, which is the failure mode a source-reading
		// gate has to rule out explicitly.
		t.Fatalf("no %s constants found in %s: this gate is not reading what it thinks it is",
			reviewOutcomeType, reviewOutcomeFile)
	}
	translated := translatedReviewOutcomes(t)

	for name, value := range declared {
		if !translated[name] {
			t.Errorf("activities.%s is %q and %s in %s has no case for it:\n"+
				"a message settling that way would fail its own transaction.\n"+
				"Add the case, returning the consent.ClosedBy… that says what the row should read.",
				name, value, reviewClosureFunc, reviewClosureFile)
		}
	}
	// The other direction, which catches a case left behind by a RENAMED
	// constant: it still compiles, still reads like coverage, and matches
	// nothing the vocabulary produces any more.
	for name := range translated {
		if _, known := declared[name]; !known {
			t.Errorf("%s in %s has a case for activities.%s, which is not a declared %s:\n"+
				"either the constant was renamed and this case was left behind, or the case is a typo.",
				reviewClosureFunc, reviewClosureFile, name, reviewOutcomeType)
		}
	}
}

// declaredReviewOutcomes reads the constant names and their string values.
//
// BY THE CONSTANT'S TYPE rather than by a name prefix: a prefix match would
// miss an outcome somebody named differently, and missing one is exactly the
// failure this gate is for.
func declaredReviewOutcomes(t *testing.T) map[string]string {
	t.Helper()
	file := parseGateSource(t, reviewOutcomeFile)
	out := map[string]string{}
	for _, decl := range file.Decls {
		gen, ok := decl.(*ast.GenDecl)
		if !ok || gen.Tok != token.CONST {
			continue
		}
		// A const block states its type once, on the first spec, and the rest
		// inherit it. Carrying it forward is what lets the block be written the
		// ordinary way instead of repeating the type on every line.
		typed := false
		for _, spec := range gen.Specs {
			value, ok := spec.(*ast.ValueSpec)
			if !ok {
				continue
			}
			if ident, ok := value.Type.(*ast.Ident); ok {
				typed = ident.Name == reviewOutcomeType
			} else if value.Type != nil {
				typed = false
			}
			if !typed {
				continue
			}
			for i, name := range value.Names {
				if i >= len(value.Values) {
					continue
				}
				if literal, ok := constantStringOf(value.Values[i]); ok {
					out[name.Name] = literal
				}
			}
		}
	}
	return out
}

// translatedReviewOutcomes reads the constant NAMES the map's switch has cases
// for.
//
// BY NAME, NOT BY VALUE, and the difference is the whole worth of the reverse
// check. Resolving each case back through the vocabulary to a value and
// comparing values makes both directions ask the same question: a constant
// whose VALUE was edited keeps its name, so the case still resolves and the
// gate reports coverage that does not exist. Names are what the switch actually
// writes and what a rename actually changes.
func translatedReviewOutcomes(t *testing.T) map[string]bool {
	t.Helper()
	file := parseGateSource(t, reviewClosureFile)
	out := map[string]bool{}
	found := false
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != reviewClosureFunc || fn.Body == nil {
			continue
		}
		found = true
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			clause, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, expr := range clause.List {
				// activities.ReviewOutcomeCancelled — the selector's field is
				// the constant name, which the vocabulary above resolves to the
				// value the switch will actually compare against.
				sel, ok := expr.(*ast.SelectorExpr)
				if !ok {
					continue
				}
				// The package qualifier is checked so a case on some other
				// package's identically-named constant is not read as
				// coverage of this vocabulary.
				pkg, ok := sel.X.(*ast.Ident)
				if !ok || pkg.Name != reviewOutcomePackage {
					continue
				}
				out[sel.Sel.Name] = true
			}
			return true
		})
	}
	if !found {
		t.Fatalf("%s not found in %s: this gate is checking a function that no longer exists",
			reviewClosureFunc, reviewClosureFile)
	}
	return out
}

// TestTheClosureMapRefusesWhatItDoesNotKnow pins the other half of the gate
// above: that an unmapped outcome is REFUSED rather than quietly closed as
// something else.
//
// Without this, a default arm returning a closure would satisfy the coverage
// check by making every case redundant — and a new outcome would then close its
// review with the wrong account of why the message stopped, which is worse than
// the error it replaced.
func TestTheClosureMapRefusesWhatItDoesNotKnow(t *testing.T) {
	t.Parallel()

	file := parseGateSource(t, reviewClosureFile)
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Name.Name != reviewClosureFunc || fn.Body == nil {
			continue
		}
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			clause, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			// A case clause with no expressions IS the default arm.
			if len(clause.List) == 0 {
				t.Errorf("%s in %s has a default arm:\n"+
					"every outcome must be named, so an unmapped one is refused rather than "+
					"closed as whatever the default returns.\n"+
					"Delete the default and let the function fall through to its refusal.",
					reviewClosureFunc, reviewClosureFile)
			}
			return true
		})
	}
}

// parseGateSource reads one file out of the tree this gate walks.
func parseGateSource(t *testing.T, path string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.ParseComments)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return file
}

// constantStringOf unquotes a string literal, and answers false for anything
// else — a computed value is not a vocabulary member this gate can read.
func constantStringOf(expr ast.Expr) (string, bool) {
	literal, ok := expr.(*ast.BasicLit)
	if !ok || literal.Kind != token.STRING {
		return "", false
	}
	unquoted, err := strconv.Unquote(literal.Value)
	if err != nil {
		return "", false
	}
	return unquoted, true
}
