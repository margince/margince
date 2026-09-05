// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// Converting money to the base currency happens in ONE place, and this fails
// when a second appears.
//
// The rule has four parts — which rate, what a missing one means, what an
// unrepresentable product means, and the two currencies' minor-unit scales —
// and it was two hand-written copies before, agreeing by inspection. They
// stopped agreeing on the fourth: a stored rate is a MAJOR-unit rate while the
// amounts count MINOR units, and JPY carries no minor unit where EUR carries
// two, so one copy read five million yen as three hundred euros while the other
// read it as thirty thousand. Both surfaces published the figure under the same
// field name.
//
// So `deals.ConvertToBase` is the only multiply, and every Go caller reaches it
// — directly or through `deals.PriceAll`. A new site that multiplies an amount
// by an `FXRate` itself is what this catches, because that site will not carry
// the scales and nothing else would notice.
//
// WHAT THIS CANNOT SEE, stated so the next reader does not trust it further
// than it goes: the SQL spellings of the same rule. There are THREE, not one —
// organization_open_pipeline_rollup, compose.BaseValueSQL and its
// character-identical twin briefs.briefBaseValueSQL. All three read
// currency_minor_digits, the database mirror of the Go digit table.
//
// Their agreement is held where it can be, in the integration lane against a
// live database: the minor-unit parity test and the one-account-one-number test
// for the rollup (minorunits_integration_test.go,
// openpipelineagreement_integration_test.go), and the scale cases plus the
// closed-deal freeze for the other two (basecurrencyscale_integration_test.go),
// whose expected amounts are literal so a scale dropped from either side fails
// them. The twins are additionally held identical to each other by
// TestOneSpellingOfADealsBaseValue, which is a text comparison and would pass
// over two identically wrong copies — that is why the literal-amount cases
// exist beside it.
//
// A unit test over source text cannot execute SQL, and pretending otherwise
// here would be the census that fails short.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// fxRateFieldReads names the expression a conversion site must reach for: the
// stored numeric off an FXRate. A site that touches it and does not hand it to
// ConvertToBase is doing the arithmetic itself.
const fxRateField = "Rate"

// convertEntryPoints are the two names a caller may reach for. PriceAll is a
// wrapper over ConvertToBase, so a caller of either is inside the one rule.
var convertEntryPoints = map[string]bool{"ConvertToBase": true, "PriceAll": true}

// fxConversionOwner is where the arithmetic itself lives, and the one file
// exempt from the rule below.
const fxConversionOwner = "internal/modules/deals/fxconvert.go"

func TestOnlyOnePlaceMultipliesAnAmountByAStoredRate(t *testing.T) {
	t.Parallel()
	offenders := []string{}
	scanned := 0

	for _, root := range []string{"internal", "cmd", "../extensions"} {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			if strings.HasSuffix(path, "_test.go") || strings.Contains(filepath.ToSlash(path), fxConversionOwner) {
				return nil
			}
			file, parseErr := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
			if parseErr != nil {
				return parseErr
			}
			scanned++
			offenders = append(offenders, rateArithmeticIn(file, path)...)
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}

	// A census that reads nothing reports PASS. The tree has hundreds of Go
	// files under these roots, so a scan that found a handful means the walk
	// broke rather than the tree shrank.
	if scanned < 100 {
		t.Fatalf("scanned only %d files — the walk is not reaching the tree, and a census that "+
			"reads a smaller tree agrees with everything in the part it missed", scanned)
	}
	for _, offender := range offenders {
		t.Errorf("%s multiplies an amount by a stored rate itself.\n"+
			"Call deals.ConvertToBase (or deals.PriceAll) instead: it applies BOTH currencies' "+
			"minor-unit scales, and a hand-written multiply does not — which reads a yen amount "+
			"as a euro one and is invisible until an installation prices a deal in JPY, KRW or VND.", offender)
	}
}

// rateArithmeticIn reports every binary multiplication in one file whose
// operand is an FXRate's stored numeric.
//
// It matches the EXPRESSION rather than a line or an import, because an import
// is present in files that never convert and absent from one that took the
// rate as a parameter. Matching statements, not lines, is what keeps this from
// failing short.
func rateArithmeticIn(file *ast.File, path string) []string {
	found := []string{}
	ast.Inspect(file, func(node ast.Node) bool {
		call, isCall := node.(*ast.CallExpr)
		if isCall && reachesTheEngine(call) {
			// The sanctioned door. Its arguments are not searched further:
			// handing rate.Rate to ConvertToBase is the correct site.
			return false
		}
		binary, isBinary := node.(*ast.BinaryExpr)
		if !isBinary || binary.Op != token.MUL {
			return true
		}
		if namesRateField(binary.X) || namesRateField(binary.Y) {
			found = append(found, path)
		}
		return true
	})
	return found
}

// reachesTheEngine reports whether a call is one of the sanctioned entry
// points, by the selector's own name — deals.ConvertToBase from outside the
// module, ConvertToBase from within it.
func reachesTheEngine(call *ast.CallExpr) bool {
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		return convertEntryPoints[fun.Sel.Name]
	case *ast.Ident:
		return convertEntryPoints[fun.Name]
	}
	return false
}

// namesRateField reports whether an expression reads an FXRate's `.Rate`
// ANYWHERE inside it, not only as its outermost node.
//
// The whole subtree is searched because the first mutation written against this
// gate reached the same value one step deeper — `rate.Rate.Int.Int64()` rather
// than `rate.Rate` — and a matcher that only looked at the top node reported
// PASS over a planted second multiply. That is the direction a census must not
// fail in, so the shape of the read is what this matches, not its depth.
func namesRateField(expr ast.Expr) bool {
	found := false
	ast.Inspect(expr, func(node ast.Node) bool {
		if selector, ok := node.(*ast.SelectorExpr); ok && selector.Sel.Name == fxRateField {
			found = true
			return false
		}
		return !found
	})
	return found
}
