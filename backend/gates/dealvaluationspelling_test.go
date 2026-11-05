// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A frozen-only sum loses open foreign-currency and legacy won values.
// Deal readers must compose the shared valuation instead.

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

var (
	frozenValueSum      = regexp.MustCompile(`(?is)\bsum\s*\([^)]*\bamount_minor_base\b`)
	currencyValueBranch = regexp.MustCompile(`(?is)\bwhen\s+[a-z_]*\.?currency\s*=\s*[^\s]*\s+then\s+[a-z_]*\.?amount_minor\b`)
)

func TestDealTotalsUseTheSharedBaseValuation(t *testing.T) {
	t.Parallel()
	scanned := 0
	for _, path := range handWrittenGoSources(t) {
		if strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
			continue
		}
		if filepath.ToSlash(path) == "internal/modules/deals/basevalue.go" {
			continue
		}
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			t.Fatal(err)
		}
		scanned++
		if bypassesDealValuation(file) {
			t.Errorf("%s values deals without deals.BaseValueSQL; frozen-only totals omit open and legacy deal values", path)
		}
	}
	if scanned < 100 {
		t.Fatalf("only %d files scanned; the valuation census missed its subjects", scanned)
	}
}

func bypassesDealValuation(file *ast.File) bool {
	for _, sql := range gatekit.SQLStatementsOf(file) {
		if frozenValueSum.MatchString(sql) || currencyValueBranch.MatchString(sql) {
			return true
		}
	}
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if !ok || len(call.Args) < 5 {
			return true
		}
		callee, ok := call.Fun.(*ast.SelectorExpr)
		if !ok || callee.Sel.Name != "MaskedColumnSQL" {
			return true
		}
		if literal, ok := call.Args[4].(*ast.BasicLit); ok && literal.Kind == token.STRING {
			value, valid := gatekit.LiteralText(literal)
			found = found || valid && value == "amount_minor_base"
		}
		return true
	})
	return found
}

func TestTheValuationCensusSeesFrozenAndComposedCopies(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, source string
		want         bool
	}{
		{"frozen sum", "var q = `SELECT sum(d.amount_minor_base) FROM deal d`", true},
		{"split sum", "var q = `SELECT su` + `m(d.amount_minor_base) FROM deal d`", true},
		{"masked frozen column", `func read() { auth.MaskedColumnSQL(ctx, "deal", "amount_minor", "d", "amount_minor_base", arg) }`, true},
		{"native-only branch", "var q = `CASE WHEN d.currency = ` + base + ` THEN d.amount_minor ELSE d.amount_minor_base END`", true},
		{"shared owner", `var q = deals.BaseValueSQL(asOf, base, "d")`, false},
		{"ordinary native sum", "var q = `SELECT sum(d.amount_minor) FROM deal d GROUP BY currency`", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			file, err := parser.ParseFile(token.NewFileSet(), "probe.go", "package probe\n"+tc.source, 0)
			if err != nil {
				t.Fatal(err)
			}
			if got := bypassesDealValuation(file); got != tc.want {
				t.Errorf("detects copy = %v, want %v", got, tc.want)
			}
		})
	}
}
