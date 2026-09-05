// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// Every path that renders a report spec's population applies that spec's row
// narrowings.
//
// This gate exists because one did not. The generic analytics compiler built
// its WHERE from the spec's base predicate and the caller's filters and
// stopped, so it shipped without the activity content clause — the thing that
// enforces `restricted_at`, the link-reachability walk, and the audience a
// human set on a thread. The audience arm does not yield to row_scope=all, so
// an administrator read private mail through a count, and every gate was green.
//
// The rule the comment on specScopeClauses states is that it is EVERY row-level
// narrowing a spec carries. This holds it: a second function that reaches for
// one of those clauses directly is building a second, partial answer.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// scopeClauseOwner names the composer this gate reports against. That it is the
// only composer is what the test below establishes, so the claim is held by the
// file it sits in rather than asserted here.
const scopeClauseOwner = "specNarrowings"

// narrowingCalls are the clause builders a population's WHERE is made of.
// Calling one outside the owner is the shape this gate reports.
var narrowingCalls = map[string]bool{
	"ActivityContentClause": true,
	"ScopeClauseFor":        true,
	"maskExclusionClauses":  true,
	// The scope HALF counts as a partial answer. Calling it instead of the
	// whole composer is exactly the defect this gate exists for — it was the
	// first thing tried against this gate and it passed, because the waiver
	// keyed on the callee's own name rather than on who called it.
	"specScopeClauses": true,
	// The POPULATION is a narrowing like the others, and the newest: a spec
	// whose row scope renders TRUE — a deal is an identity table — is narrowed
	// by nothing else, so a path that skipped this answered every caller the
	// whole installation. That is how a rep's Pipeline disagreed with their own
	// Forecast.
	"AnalyticsPopulationClause": true,
}

// composesItsOwnNarrowing ratifies the spec-taking functions that legitimately
// call a clause builder directly. Each says which different question it asks.
var composesItsOwnNarrowing = gatekit.Waive(map[string]string{
	"buildReportWhere": "the report engine's own WHERE builder: it takes the scope and " +
		"population halves here and the mask and reference halves in fetchRows beside it, " +
		"because fetchRows also COUNTS the masked rows and needs those clauses separately. " +
		"Between the two, all four are applied",
	"fetchRows": "the report engine, which composes the three itself because it also " +
		"COUNTS the rows the mask withheld for excluded_by_permission and so needs those " +
		"clauses in hand rather than folded into a list. It applies all three",
	"fetchDerivation": "the drill-through, which must read the IDENTICAL row set the " +
		"aggregate read — including the same mask exclusions — and composes them beside " +
		"the group-key predicates that make it a drill-through",
	"derivationWhere": "the drill-through's own predicate builder, the sibling of " +
		"buildReportWhere for a bound group key — and it takes the SAME population, or " +
		"the explanation opens records the number never counted",
	"referenceScopeClauses": "a DIFFERENT question: the row scope over the records a " +
		"population POINTS AT, asked per declared reference rather than over the " +
		"population's own rows. It is called alongside specScopeClauses, never instead",
	"maskedDerivationSelects": "not a narrowing at all: it decides which COLUMN of a row " +
		"the drill-through already returns comes back NULL, and adds no predicate. The rows " +
		"it selects over are derivationWhere's, unchanged — a row whose reference this blanks " +
		"is still counted by the aggregate beside it, which is the point",
})

func TestEveryPopulationsNarrowingsAreComposedInOnePlace(t *testing.T) {
	t.Parallel()
	defer composesItsOwnNarrowing.AssertAllMatched(t)

	fset := token.NewFileSet()
	var offences []string
	judged := 0
	for _, path := range handWrittenGoSources(t) {
		where := filepath.ToSlash(path)
		// The report engine and the analytics compiler are the subject. Other
		// tiers narrow their own reads and are not building a population from
		// a report spec.
		if !strings.HasPrefix(where, "internal/compose/") || strings.HasSuffix(where, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", where, err)
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok {
				continue
			}
			// The corpus is the functions that BUILD FROM A SPEC. Every other
			// read in this tier narrows its own query and is not composing a
			// report population — sweeping those in would ratify dozens of
			// files this gate has not judged, which is how a waiver list
			// becomes the thing it was meant to prevent.
			if !takesReportSpec(fn) {
				continue
			}
			judged++
			// The composer itself, and the half it is made of.
			if fn.Name.Name == scopeClauseOwner || fn.Name.Name == "specScopeClauses" {
				continue
			}
			if calledNarrowing(fn) == "" {
				continue
			}
			if composesItsOwnNarrowing.Waived(t, fn.Name.Name) {
				continue
			}
			offences = append(offences, where+": "+fn.Name.Name+
				" calls "+calledNarrowing(fn)+" directly")
		}
	}
	// A census that can fail short has already failed: reading no functions
	// would report a clean pass over nothing.
	if judged == 0 {
		t.Fatal("this gate found no spec-taking function in internal/compose — the corpus " +
			"selector has stopped reaching its subject, and a pass over nothing is what " +
			"it would report next")
	}
	for _, offence := range offences {
		t.Errorf(`%s.

A population's row narrowings are composed in %s, which is what makes them
complete. Composing a subset elsewhere is how the analytics compiler shipped
without the activity content clause, and read private threads through a count.

Call %s, or ratify this function in composesItsOwnNarrowing saying which
different question it asks.`, offence, scopeClauseOwner, scopeClauseOwner)
	}
}

// takesReportSpec answers whether a function is handed a reportSpec, which is
// what makes it a builder of a report population rather than an ordinary read.
func takesReportSpec(fn *ast.FuncDecl) bool {
	if fn.Type.Params == nil {
		return false
	}
	for _, param := range fn.Type.Params.List {
		if ident, ok := param.Type.(*ast.Ident); ok && ident.Name == "reportSpec" {
			return true
		}
	}
	return false
}

// calledNarrowing answers the first clause builder a function calls, or "".
func calledNarrowing(fn *ast.FuncDecl) string {
	found := ""
	ast.Inspect(fn, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := ""
		switch target := call.Fun.(type) {
		case *ast.SelectorExpr:
			name = target.Sel.Name
		case *ast.Ident:
			name = target.Name
		}
		if narrowingCalls[name] {
			found = name
			return false
		}
		return true
	})
	return found
}
