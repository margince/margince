// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

//go:build !integration

package gates

// Converting money to the base currency has one implementation.
//
// It had two, in two languages: exact decimal over big.Int in the hierarchy
// rollup, and a LEFT JOIN LATERAL on fx_rate with round(amount × rate)::bigint
// on the company page. Both encoded the same four decisions — the direction,
// the as-of cutoff, newest-wins, and the multiply-and-round — and they agreed.
//
// Nothing made them keep agreeing. The predicted first divergence was the
// rounding: half-away-from-zero against Postgres round(), which is a
// one-minor-unit disagreement between two pages about the same account, and the
// kind of defect nobody can reproduce on demand.
//
// What is NOT shared, and must not be, is the missing-rate policy: the rollup
// refuses the whole read, the company page prices what it can and counts the
// rest. Both are right for their own surface, so the engine answers "is there a
// rate, and what does it make of this amount" and leaves the rest to the caller.
//
// This gate is about the ENGINE, so it hunts a second one: a read of fx_rate
// outside the module that owns it, and an arithmetic conversion spelled in SQL.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// fxRateOwner is the module whose store owns fx_rate, and so the one place the
// lookup is written. tableOwners agrees; this names it for the failure message.
const fxRateOwner = "internal/modules/deals"

// readsFXRate matches a statement that goes to the rate table itself.
var readsFXRate = regexp.MustCompile(`(?i)\bfrom\s+fx_rate\b|\bjoin\s+fx_rate\b`)

// convertsInSQL matches the arithmetic: an amount multiplied by a rate. It is
// the half that makes a second read a second ENGINE rather than a lookup — a
// query that reads a rate and hands it to Go is converting through
// deals.ConvertToBase like everything else.
var convertsInSQL = regexp.MustCompile(`(?i)amount_minor\s*\*|\*\s*[a-z_.]*rate\b`)

// fxConversionExempt ratifies the files that may spell either half.
var fxConversionExempt = gatekit.Waive(map[string]string{
	"internal/modules/deals/fxconvert.go": "the engine itself: the one lookup and the one arithmetic every " +
		"caller reaches money conversion through",
	"internal/modules/deals/fxrate_store.go": "the rate table's own store — reading and writing the rows the " +
		"engine later looks up is what owning the table means",
	"internal/modules/deals/basecurrencyfreeze.go": "the CLOSE-time freeze, which is a different question: a " +
		"closed deal stores the rate it was converted at, where this engine answers what an OPEN one converts " +
		"at today",
	"internal/modules/deals/deal_advance.go": "the same freeze at the moment it happens — advancing to won " +
		"reads the rate once and stores it on the deal, which is why a closed figure never moves again",
	"internal/modules/quotas/attainment.go": "a THIRD conversion, and one the module DAG forces: quotas is a " +
		"module and a module never imports a sibling (ADR-0054 §3), so it cannot reach deals.FXRates and must " +
		"ask fx_rate itself. Its own comment already names the two implementations it mirrors. Holding it " +
		"identical to them is an obligation this gate cannot discharge — what it can do is stop a FOURTH " +
		"appearing anywhere the engine IS reachable",
	"internal/compose/rateproposals_integration_test.go": "a test seeding and asserting on rate rows. It reads " +
		"the table to arrange and to check, never to convert an amount for a reader",
	"internal/compose/briefs/briefrank.go": "a FOURTH conversion, and the only ratified one that could reach " +
		"the engine — it is in compose. It converts inside a larger ranking query and answers a wider " +
		"question than the engine does: a CLOSED deal reads its frozen amount_minor_base, which is a stored " +
		"figure rather than a conversion. Moving it means restructuring that query and teaching the engine " +
		"the frozen case, which is its own change rather than a line in this one. Recorded so it is a known " +
		"copy rather than an unnoticed one",
})

func TestMoneyConvertsToTheBaseCurrencyInOnePlace(t *testing.T) {
	t.Parallel()
	defer fxConversionExempt.AssertAllMatched(t)

	var offences []string
	judged := 0
	// The WHOLE tree, walked directly rather than through a Scope: a second
	// conversion is only interesting where nobody thought to look, and the
	// roots a gate names are exactly where it did.
	fset := token.NewFileSet()
	for _, path := range handWrittenGoSources(t) {
		where := filepath.ToSlash(path)
		file, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			t.Fatalf("parsing %s: %v", path, err)
		}
		literals := sqlLiteralsIn(file)
		for _, sql := range literals {
			if !readsFXRate.MatchString(sql) {
				continue
			}
			judged++
			if fxConversionExempt.Waived(t, where) {
				continue
			}
			if strings.HasPrefix(where, fxRateOwner+"/") {
				offences = append(offences, where+" reads fx_rate outside the engine, inside the module that "+
					"owns it — the lookup is deals.FXRates")
				continue
			}
			offences = append(offences, where+" reads fx_rate directly; the lookup is deals.FXRates, which "+
				"memoizes one per currency and answers the same rate the other surface converts at")
		}
		for _, sql := range literals {
			if !convertsInSQL.MatchString(sql) || !readsFXRate.MatchString(sql) {
				continue
			}
			if fxConversionExempt.Waived(t, where) {
				continue
			}
			offences = append(offences, where+" multiplies an amount by a rate in SQL; the arithmetic is "+
				"deals.ConvertToBase, which rounds half away from zero over exact decimal digits")
		}
	}
	// A census that judged nothing certifies nothing: the rate table is read by
	// the engine and its store at minimum, so a zero here is a broken scan.
	if judged == 0 {
		t.Fatal("no statement reading fx_rate was found at all, so this gate is reading a tree shape " +
			"that is gone rather than a clean one")
	}
	if len(offences) > 0 {
		t.Errorf("money converts to the base currency in more than one place:\n  %s\n\n"+
			"Two implementations of one conversion agree until they do not, and the first thing to "+
			"diverge is the rounding — a one-minor-unit disagreement between two pages about the same "+
			"account, which nobody can reproduce on demand.", strings.Join(offences, "\n  "))
	}
}

// sqlLiteralsIn returns every string literal in a file, which is where a
// statement lives. Concatenations are read piece by piece: a fragment naming
// fx_rate is a read of it whichever half of the concatenation it sits in.
func sqlLiteralsIn(file *ast.File) []string {
	var out []string
	ast.Inspect(file, func(node ast.Node) bool {
		if lit, ok := node.(*ast.BasicLit); ok {
			if text, isText := gatekit.LiteralText(lit); isText {
				out = append(out, text)
			}
		}
		return true
	})
	return out
}
