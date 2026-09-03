// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H2

package gates

// One deal's base-currency value is spelled twice, in two packages that cannot
// import each other, and this is what stops the two from drifting.
//
// The drift that matters is not cosmetic. The expression decides three things a
// reader would never suspect were decisions: that a closed deal keeps the rate
// it closed at rather than being re-converted, that an open one takes the
// latest rate on or before the as-of day, and that a missing rate is NULL and
// never 1. Change any of them in one copy and a forecast headline and a morning
// brief disagree about the same deal, with nothing failing anywhere.
//
// The comparison is on the RENDERED SQL and not on the source text, because
// what must match is what Postgres executes. Two functions could be spelled
// differently and render the same statement, and that is fine; two spelled
// identically but rendered with different bind positions is the defect.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"
)

// The two copies, by the file that holds each and the function that renders it.
const (
	baseValueOwner  = "internal/compose/basecurrencysql.go"
	baseValueOwnFn  = "BaseValueSQL"
	baseValueTwin   = "internal/compose/briefs/briefrank.go"
	baseValueTwinFn = "briefBaseValueSQL"
)

func TestOneSpellingOfADealsBaseValue(t *testing.T) {
	t.Parallel()
	own := baseValueFormat(t, baseValueOwner, baseValueOwnFn)
	twin := baseValueFormat(t, baseValueTwin, baseValueTwinFn)
	if own != twin {
		t.Errorf(`the two spellings of a deal's base-currency value have diverged.

%s renders:
%s

%s renders:
%s

They are one rule and must stay one. Change both, or move the pair to a tier
both packages can import and delete this gate.`,
			baseValueOwner, own, baseValueTwin, twin)
	}
}

// baseValueFormat extracts the format string the named function passes to
// fmt.Sprintf.
//
// It FAILS rather than returning empty when it cannot find one. A census that
// can fail short has already failed: an empty string compared against another
// empty string passes, reports nothing, and leaves no assertion to notice that
// the gate stopped reading either copy.
func baseValueFormat(t *testing.T, path, fn string) string {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), path, nil, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	for _, decl := range file.Decls {
		fd, ok := decl.(*ast.FuncDecl)
		if !ok || fd.Name.Name != fn {
			continue
		}
		var format string
		ast.Inspect(fd, func(n ast.Node) bool {
			lit, ok := n.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING || !strings.Contains(lit.Value, "amount_minor") {
				return true
			}
			unquoted, err := strconv.Unquote(lit.Value)
			if err != nil {
				t.Fatalf("%s: %s holds a string literal that will not unquote: %v", path, fn, err)
			}
			format = unquoted
			return false
		})
		if format == "" {
			t.Fatalf("%s: %s no longer holds the base-value expression — this gate has stopped reading it, and a gate that reads nothing passes for the wrong reason", path, fn)
		}
		return normalizeSQL(format)
	}
	t.Fatalf("%s no longer declares %s — repoint this gate at whatever renders the expression now", path, fn)
	return ""
}
