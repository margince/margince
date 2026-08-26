// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package backendarch

// The ceiling on a statement whose predicate the CALLER wrote is one number,
// declared in platform/database as CallerPredicateBudget.
//
// It was two. The filter preview and the search query plan each declared five
// seconds, each with its own paragraph arguing for it from the same fact —
// somebody is waiting on the answer — and neither could see the other move.
// Two writers of one invariant drift by having one of them edited, and a
// preview that waits ten seconds while a plan waits five is a difference no
// reader of either file can notice.
//
// The subject is a DURATION CONSTANT WORTH FIVE SECONDS whose name says it is
// a budget. That pairing is what separates this rule from every other ceiling
// in the tree: the lane budgets, the perf floors and the probe deadlines are
// all real numbers answering different questions, and a gate that reported
// them would be waived into uselessness inside a week.
//
// The value is EVALUATED, not matched as text. `5 * time.Second` and
// `5000 * time.Millisecond` are the same ceiling, and a detector that knew only
// the spelling it was written for would let the respelling through — which is
// the direction a census must not fail in.
//
// WHAT IT CANNOT SEE. A five-second budget under a name that does not say
// "budget" (`previewCeiling`, `planDeadline`), one assembled from another
// constant, or one written as a bare literal at the call site rather than
// declared. It is a net under the shape the tree reaches for, not a proof; the
// call-site half is held by the type — BoundStatement takes a Duration and
// every production caller passes a named constant.

import (
	"fmt"
	"go/ast"
	"go/token"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// callerPredicateBudget is the one ceiling, repeated here because a gate that
// read it from the package it guards would agree with any edit to it. This is
// the declared mirror, and TestTheGateAgreesWithTheConstantItGuards fails if
// the two ever differ.
const callerPredicateBudget = 5 * time.Second

// callerPredicateBudgetScope claims the number lives in platform/database and
// nowhere else. Nothing is exempt: the package sits below every tier that asks
// a caller-written predicate, so every caller can reach the constant.
var callerPredicateBudgetScope = gatekit.Scope{
	Roots:   []string{"internal/platform/database"},
	Subject: declaresACallerPredicateBudget,
	Exempt:  gatekit.Waive(map[string]string{}),
}

func TestTheCallerPredicateBudgetIsDeclaredOnce(t *testing.T) {
	// EXACTLY one DECLARATION, not one file. A file is the wrong unit twice
	// over: two declarations in one file are two numbers and would report as
	// one, and zero of them reads the same as a clean tree — the direction a
	// census must not fail in.
	total := 0
	var where []string
	for _, f := range callerPredicateBudgetScope.Files(t) {
		n, unreadable := countCallerPredicateBudgets(f.File)
		for _, why := range unreadable {
			t.Errorf("%s declares %s — this census cannot judge it either way, so it would be counted as "+
				"absent. Write the duration in a form the reader understands", f.Path, why)
		}
		total += n
		where = append(where, fmt.Sprintf("%s (%d)", f.Path, n))
	}
	if total != 1 {
		sites := "(no file under internal/platform/database declares one)"
		if len(where) > 0 {
			sites = strings.Join(where, "\n\t")
		}
		t.Errorf("a five-second caller-predicate budget is declared %d time(s):\n\t%s\n\nOne number, so a "+
			"preview and a plan cannot come to disagree about how long somebody is made to wait. Take "+
			"database.CallerPredicateBudget", total, sites)
	}
}

// TestTheGateAgreesWithTheConstantItGuards keeps the mirror above honest: a
// gate holding its own copy of the number it protects has become the second
// copy it exists to forbid.
func TestTheGateAgreesWithTheConstantItGuards(t *testing.T) {
	if callerPredicateBudget != database.CallerPredicateBudget {
		t.Errorf("this gate measures %s and platform/database declares %s; the gate is guarding a number "+
			"the tree no longer uses", callerPredicateBudget, database.CallerPredicateBudget)
	}
}

// declaresACallerPredicateBudget reports whether a file declares a duration
// constant worth the caller-predicate ceiling under a name that says budget.
func declaresACallerPredicateBudget(_ string, file *ast.File) bool {
	total, _ := countCallerPredicateBudgets(file)
	return total > 0
}

// countCallerPredicateBudgets counts them, because the file is the wrong unit:
// two in one file are two numbers that can drift apart.
func countCallerPredicateBudgets(file *ast.File) (int, []string) {
	total := 0
	var unreadable []string
	ast.Inspect(file, func(n ast.Node) bool {
		spec, ok := n.(*ast.ValueSpec)
		if !ok {
			return true
		}
		for at, name := range spec.Names {
			if !strings.Contains(strings.ToLower(name.Name), "budget") || at >= len(spec.Values) {
				continue
			}
			d, ok, err := durationOf(spec.Values[at])
			if err != nil {
				unreadable = append(unreadable, fmt.Sprintf("%s: %v", name.Name, err))
				continue
			}
			if ok && d == callerPredicateBudget {
				total++
			}
		}
		return true
	})
	return total, unreadable
}

// durationOf evaluates `<n> * time.<Unit>` — the only shape a duration constant
// is written in here — and reports the duration it names. Evaluating rather
// than matching is what makes 5000*time.Millisecond the same subject as
// 5*time.Second; a text comparison loses to the equivalent spelling.
func durationOf(expr ast.Expr) (time.Duration, bool, error) {
	product, ok := expr.(*ast.BinaryExpr)
	if !ok || product.Op != token.MUL {
		return 0, false, nil
	}
	count, unit := product.X, product.Y
	if _, isLiteral := count.(*ast.BasicLit); !isLiteral {
		count, unit = product.Y, product.X
	}
	literal, ok := count.(*ast.BasicLit)
	if !ok || literal.Kind != token.INT {
		return 0, false, nil
	}
	n, err := strconv.ParseInt(literal.Value, 0, 64)
	if err != nil {
		// A literal too large for an int64 is not "not a duration" — it is a
		// duration this reader cannot judge, and either answer it could give
		// is a lie. Returning MaxInt64 was the first attempt and was worse
		// than false: the count compares against five seconds, so the
		// declaration was excluded anyway and the census reported it ABSENT.
		return 0, false, fmt.Errorf("a duration literal this reader cannot judge: %q: %w", literal.Value, err)
	}
	scale, ok := durationUnit(unit)
	if !ok {
		return 0, false, nil
	}
	return time.Duration(n) * scale, true, nil
}

// durationUnit reads `time.Second` and its neighbours off the selector.
func durationUnit(expr ast.Expr) (time.Duration, bool) {
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return 0, false
	}
	pkg, ok := selector.X.(*ast.Ident)
	if !ok || pkg.Name != "time" {
		return 0, false
	}
	switch selector.Sel.Name {
	case "Nanosecond":
		return time.Nanosecond, true
	case "Microsecond":
		return time.Microsecond, true
	case "Millisecond":
		return time.Millisecond, true
	case "Second":
		return time.Second, true
	case "Minute":
		return time.Minute, true
	case "Hour":
		return time.Hour, true
	}
	return 0, false
}

// TestTheBudgetCensusStillSeesItsSubject is the vacuity check: a census that
// has stopped matching passes by finding nothing, which is the same word it
// prints over a clean tree.
func TestTheBudgetCensusStillSeesItsSubject(t *testing.T) {
	subjects := map[string]string{
		"the ceiling as the tree spells it": "" +
			"package p\nimport \"time\"\nconst previewStatementBudget = 5 * time.Second\n",
		"the same ceiling in the other unit": "" +
			"package p\nimport \"time\"\nconst planStatementBudget = 5000 * time.Millisecond\n",
		"the factors written the other way round": "" +
			"package p\nimport \"time\"\nconst queryBudget = time.Second * 5\n",
		"one declared in a grouped const block": "" +
			"package p\nimport \"time\"\nconst (\n\trows = 25\n\tpreviewBudget = 5 * time.Second\n)\n",
		"one spelled with a capital B in the middle of the name": "" +
			"package p\nimport \"time\"\nvar StatementBudgetForPreview = 5 * time.Second\n",
	}
	for name, body := range subjects {
		if !declaresACallerPredicateBudget("x.go", parseGateFixture(t, body)) {
			t.Errorf("the census no longer recognises %s, so it is guarding nothing", name)
		}
	}

	nearMisses := map[string]string{
		"a lane ceiling, which is a different question with a different answer": "" +
			"package p\nimport \"time\"\nconst logoLaneBudget = 20 * time.Second\n",
		"a perf floor in milliseconds": "" +
			"package p\nimport \"time\"\nconst perf3Budget = 200 * time.Millisecond\n",
		"five seconds under a name that is not a budget": "" +
			"package p\nimport \"time\"\nconst pollInterval = 5 * time.Second\n",
		"an adopter taking the shared constant rather than declaring one": "" +
			"package p\nconst previewStatementBudget = database.CallerPredicateBudget\n",
		"a bare integer that happens to be five": "" +
			"package p\nconst retryBudget = 5\n",
	}
	for name, body := range nearMisses {
		if declaresACallerPredicateBudget("x.go", parseGateFixture(t, body)) {
			t.Errorf("the census claims %s is a second declaration of the caller-predicate ceiling; it "+
				"will be waived into uselessness if it fires on every ceiling in the tree", name)
		}
	}
}
