// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// The deal money pairing rule is decided in ONE function, and this fails when a
// second place decides it.
//
// The rule is: a deal's currency is present exactly when at least one of its
// figures is. There are two figures now — the one-off amount_minor and the
// recurring expected_arr_minor — and two writers that have to agree, the create
// path and the update path. Before the recurring figure existed each writer
// spelled the rule out for itself, as `(amount == nil) != (currency == nil)`,
// and the two copies agreed because there was only one way to write it.
//
// Adding a second figure ended that. The same shape says something different
// now: a deal with ARR and no one-off amount is legal, and either copy left
// un-updated would have refused it while the other admitted it, so the same
// request would be a 422 through create and a 200 through update. That is the
// drift this holds shut.
//
// So `deals.moneyPairError` is the only decision, both writers call it, and a
// new site that compares a money figure's nil-ness against the currency's is
// what this catches.
//
// WHAT THIS CANNOT SEE: the SQL half. deal_money_currency_pair and
// contract_money_currency_pair hold the same rule in the database, and their
// agreement with the Go copy is held in the integration lane, where the
// constraints are exercised against a live schema rather than read as text.
// A source scan cannot execute a CHECK, and claiming otherwise here would make
// this gate read as more than it is.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// moneyFigureFields are the deal's two money figures on the wire struct. A
// comparison of one of these against the currency is the rule being decided.
var moneyFigureFields = map[string]bool{
	"AmountMinor":      true,
	"ExpectedArrMinor": true,
}

// moneyCurrencyField is the other half of every such comparison.
const moneyCurrencyField = "Currency"

// dealMoneyPairOwner holds the one decision, and is the file exempt below.
const dealMoneyPairOwner = "modules/deals/dealmoney.go"

// moneyPairCallers are the two writers that must ASK the rule. Named rather
// than discovered, because the failure this catches is a call going missing,
// and a discovered list shrinks silently when one does.
var moneyPairCallers = []string{
	"internal/modules/deals/deal.go",
	"internal/modules/deals/deal_create.go",
}

func TestOneFunctionDecidesWhetherADealsMoneyPairIsLegal(t *testing.T) {
	t.Parallel()
	offenders := []string{}
	scanned := 0

	for _, root := range []string{"internal", "cmd", "../extensions"} {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
			if err != nil || entry.IsDir() || !strings.HasSuffix(path, ".go") {
				return err
			}
			if strings.HasSuffix(path, "_test.go") ||
				strings.Contains(filepath.ToSlash(path), dealMoneyPairOwner) {
				return nil
			}
			fset := token.NewFileSet()
			file, parseErr := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if parseErr != nil {
				return parseErr
			}
			scanned++
			offenders = append(offenders, moneyPairDecisionsIn(fset, file, path)...)
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}

	// A census that reads nothing reports PASS, so the walk proves it reached
	// the tree before its silence is allowed to mean anything.
	if scanned < 100 {
		t.Fatalf("scanned only %d files — the walk is not reaching the tree, and a census that "+
			"reads a smaller tree agrees with everything in the part it missed", scanned)
	}
	// The prohibition above says nobody ELSE decides the rule. It cannot say
	// that the two writers still ASK — a writer that dropped the call entirely
	// would trip no pattern and leave this gate green over a path validating
	// nothing. So the callers are asserted by name, which is the half a
	// prohibition structurally cannot cover.
	for _, caller := range moneyPairCallers {
		src, err := os.ReadFile(caller)
		if err != nil {
			t.Fatalf("reading %s: %v", caller, err)
		}
		if !strings.Contains(string(src), "moneyPairError(") {
			t.Errorf("%s no longer calls moneyPairError.\n"+
				"Both deal writers ask the one rule, or the create path and the update path "+
				"admit different rows and the same request answers differently through each.", caller)
		}
	}

	for _, offender := range offenders {
		t.Errorf("%s decides the deal money pairing rule itself.\n"+
			"Call deals.moneyPairError instead: the rule now covers two figures — the one-off "+
			"amount and the recurring ARR — so a hand-written copy admits or refuses a different "+
			"set of deals than the real one, and the same request becomes a 422 through one "+
			"writer and a 200 through the other.", offender)
	}
}

// moneyPairDecisionsIn reports every place in one file that compares a money
// figure's nil-ness against the currency's — the shape of the rule itself.
func moneyPairDecisionsIn(fset *token.FileSet, file *ast.File, path string) []string {
	found := []string{}
	ast.Inspect(file, func(n ast.Node) bool {
		cmp, ok := n.(*ast.BinaryExpr)
		if !ok || (cmp.Op != token.EQL && cmp.Op != token.NEQ) {
			return true
		}
		// `(a == nil) != (c == nil)` parses as one BinaryExpr whose two sides
		// are themselves nil tests.
		left, right := nilTestField(cmp.X), nilTestField(cmp.Y)
		if left == "" || right == "" {
			return true
		}
		if (moneyFigureFields[left] && right == moneyCurrencyField) ||
			(moneyFigureFields[right] && left == moneyCurrencyField) {
			found = append(found, fmt.Sprintf("%s:%d", filepath.ToSlash(path),
				fset.Position(cmp.Pos()).Line))
		}
		return true
	})
	return found
}

// nilTestField reads `x.Field == nil` (either order, optionally parenthesised)
// and returns the field name. Anything else returns "".
func nilTestField(e ast.Expr) string {
	for {
		paren, ok := e.(*ast.ParenExpr)
		if !ok {
			break
		}
		e = paren.X
	}
	cmp, ok := e.(*ast.BinaryExpr)
	if !ok || (cmp.Op != token.EQL && cmp.Op != token.NEQ) {
		return ""
	}
	for _, pair := range [][2]ast.Expr{{cmp.X, cmp.Y}, {cmp.Y, cmp.X}} {
		ident, isIdent := pair[1].(*ast.Ident)
		if !isIdent || ident.Name != "nil" {
			continue
		}
		if sel, isSel := pair[0].(*ast.SelectorExpr); isSel {
			return sel.Sel.Name
		}
	}
	return ""
}
