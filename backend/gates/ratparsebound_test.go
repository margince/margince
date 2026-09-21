// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A decimal string is shape-checked before math/big parses it.
//
// big.Rat's SetString takes exponent and fraction forms whose exact value is
// sized by the digits of the exponent rather than by the length of the text, so
// a short string can ask for a very large value, and the process builds it
// before any magnitude check that follows can refuse. The
// tree's one spelling of the fence is values.PlainDecimal — digits, one dot,
// bounded integer and fractional widths — and the rate stores already pass
// through it. A parse that does not was a copy that forgot, and nothing else in
// the tree would say so until a page or a payload chose the exponent.
//
// So every SetString call in hand-written product code must be preceded by a
// fence on the SAME text, or carry a waiver saying why its input cannot choose
// that shape. A fence is only what refuses: an `if` among the function's own
// top-level statements whose condition is `!PlainDecimal(x, …)`, alone or as one
// arm of an `||`, and whose body returns. A PlainDecimal whose result is
// ignored, that guards a branch which carries on, that sits in an `&&` another
// operand can short-circuit, or that checks some other string, decides nothing
// about the parse, and a census that credited it would pass by miscounting.
//
// The census matches the method name, not the receiver type: over-matching a
// big.Int or big.Float parse costs one waiver with a reason, while a
// type-directed match that missed a receiver spelling would pass by seeing
// less. The text is compared as written, so a fence on `s` does not cover a
// parse of `strings.TrimSpace(s)`.
//
// What this cannot see: a guard that lives in a CALLER rather than in the
// parsing function itself, and a variable reassigned between its fence and its
// parse. The first is reported, and either takes the fence into its own body or
// is waived with the caller named — which is the point, because the next caller
// will not know the fence was somebody else's.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"io/fs"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// plainDecimalFence is the one guard the census recognises.
const plainDecimalFence = "PlainDecimal"

// unfencedRatParseWaivers are the parses whose input cannot choose an exponent
// or fraction form, keyed file:function, each with what its input is.
var unfencedRatParseWaivers = gatekit.Waive(map[string]string{
	"internal/compose/aicert/scenariorender.go:representable": "compares a number read from a " +
		"corpus YAML file this repository authors; ParseFloat has already accepted the text, and no " +
		"request, page or model output reaches the corpus loader",
	"internal/compose/certcase_agentloop_window.go:agentLoopSameNumber": "certification-only " +
		"comparison of a fixed scenario's expected argument with the one a model produced for that " +
		"scenario; an operator runs it against their own provider, and no fetched or user content is in the prompt",
	"internal/compose/offerdraft_price.go:validDecimal": "its rune filter admits only digits, '.' " +
		"and '-' before the parse, so no exponent or fraction form reaches big.Rat and the cost stays " +
		"linear in the text's length",
	"internal/compose/rateproposals.go:sameRate": "compares a rate fxRateString already fenced, or a " +
		"numeric(20,10) value read back from the currency sheet, against another of the same; the proposal " +
		"payload it reads is written by the server from those values",
	"internal/modules/deals/offer_totals.go:ratFromDecimal": "its rune filter admits only digits, " +
		"'.' and '-' before the parse, mirroring the numeric columns it reads, so no exponent or fraction " +
		"form reaches big.Rat",
})

// unfencedRatParse is one SetString call no fence on its text precedes.
type unfencedRatParse struct {
	key string
	pos token.Position
}

// decimalFence is one refusing PlainDecimal check: the text it judged, and where the
// statement that refuses ends.
type decimalFence struct {
	text string
	end  token.Pos
}

// unfencedRatParses reports every SetString call in file that no fence on its
// argument precedes within the same top-level function, plus how many SetString
// calls it judged at all.
func unfencedRatParses(fset *token.FileSet, path string, file *ast.File) (found []unfencedRatParse, judged int) {
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		fences := refusingFences(fn.Body)
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || calledName(call) != "SetString" {
				return true
			}
			judged++
			text := ""
			if len(call.Args) == 1 {
				text = types.ExprString(call.Args[0])
			}
			if !slices.ContainsFunc(fences, func(f decimalFence) bool { return f.text == text && f.end < call.Pos() }) {
				found = append(found, unfencedRatParse{key: path + ":" + fn.Name.Name, pos: fset.Position(call.Pos())})
			}
			return true
		})
	}
	return found, judged
}

// refusingFences collects the fences among body's own top-level statements:
// only there does a returning branch provably stand between every later
// statement and a text that failed the check.
func refusingFences(body *ast.BlockStmt) []decimalFence {
	var fences []decimalFence
	for _, stmt := range body.List {
		ifStmt, ok := stmt.(*ast.IfStmt)
		if !ok || len(ifStmt.Body.List) == 0 {
			continue
		}
		if _, returns := ifStmt.Body.List[len(ifStmt.Body.List)-1].(*ast.ReturnStmt); !returns {
			continue
		}
		for _, operand := range disjuncts(ifStmt.Cond) {
			not, ok := operand.(*ast.UnaryExpr)
			if !ok || not.Op != token.NOT {
				continue
			}
			call, ok := ast.Unparen(not.X).(*ast.CallExpr)
			if ok && calledName(call) == plainDecimalFence && len(call.Args) > 0 {
				fences = append(fences, decimalFence{text: types.ExprString(call.Args[0]), end: ifStmt.End()})
			}
		}
	}
	return fences
}

// disjuncts flattens a condition into the operands of its top-level ||: each is
// sufficient on its own to take the branch.
func disjuncts(cond ast.Expr) []ast.Expr {
	cond = ast.Unparen(cond)
	if or, ok := cond.(*ast.BinaryExpr); ok && or.Op == token.LOR {
		return append(disjuncts(or.X), disjuncts(or.Y)...)
	}
	return []ast.Expr{cond}
}

// calledName is the bare name a call reaches: the selector for pkg.F and
// recv.M, the identifier for a same-package F.
func calledName(call *ast.CallExpr) string {
	switch fun := call.Fun.(type) {
	case *ast.SelectorExpr:
		return fun.Sel.Name
	case *ast.Ident:
		return fun.Name
	default:
		return ""
	}
}

// TestEveryDecimalParseIsFencedBeforeBigRatReadsIt walks the product tree and
// refuses a SetString whose function has not first passed its text through
// values.PlainDecimal.
func TestEveryDecimalParseIsFencedBeforeBigRatReadsIt(t *testing.T) {
	t.Parallel()
	defer unfencedRatParseWaivers.AssertAllMatched(t)
	fset := token.NewFileSet()
	judged := 0
	sawFxRate := false
	for _, root := range []string{"internal", "cmd"} {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil || d.IsDir() {
				return err
			}
			path = filepath.ToSlash(path)
			if !strings.HasSuffix(path, ".go") ||
				strings.HasSuffix(path, "_test.go") ||
				strings.HasSuffix(path, "_gen.go") ||
				strings.HasPrefix(path, "internal/contracts/") {
				return nil
			}
			file, err := parser.ParseFile(fset, path, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			found, n := unfencedRatParses(fset, path, file)
			judged += n
			if path == "internal/compose/fxextract.go" && n > 0 {
				sawFxRate = true
			}
			for _, parse := range found {
				if unfencedRatParseWaivers.Waived(t, parse.key) {
					continue
				}
				t.Errorf("%s: SetString with no values.PlainDecimal before it in %s — an exponent or "+
					"fraction form sizes the parsed value by its digits, so fence the text first or waive "+
					"the function with what its input is", parse.pos, parse.key)
			}
			return nil
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	// fxRateString is the parse this gate exists for, so a walk that did not
	// judge it is not seeing the tree it reports on.
	if !sawFxRate || judged == 0 {
		t.Fatalf("judged %d SetString calls and fxextract.go's was %v among them — the walk is not "+
			"reading the tree it claims to", judged, sawFxRate)
	}
}

// TestTheDecimalFenceCensusRefusesTheShapesItExistsFor plants the defects the
// census must see, so a recogniser that stopped matching fails here rather than
// passing over the whole tree.
func TestTheDecimalFenceCensusRefusesTheShapesItExistsFor(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, body string
		want       int
	}{
		{"unfenced", `r, _ := new(big.Rat).SetString(s); _ = r`, 1},
		{"fenced first", `if !values.PlainDecimal(s, 10, 10) { return }; r, _ := new(big.Rat).SetString(s); _ = r`, 0},
		{"fenced as one arm of an or", `if s == "" || !values.PlainDecimal(s, 10, 10) { return }; r, _ := new(big.Rat).SetString(s); _ = r`, 0},
		{"fence after the parse", `r, _ := new(big.Rat).SetString(s); if !values.PlainDecimal(s, 10, 10) { return }; _ = r`, 1},
		{"parse inside a closure", `f := func() { var r big.Rat; r.SetString(s) }; f()`, 1},
		{"result ignored", `_ = values.PlainDecimal(s, 10, 10); r, _ := new(big.Rat).SetString(s); _ = r`, 1},
		{"a different text fenced", `if !values.PlainDecimal(t, 10, 10) { return }; r, _ := new(big.Rat).SetString(s); _ = r`, 1},
		{"branch that carries on", `if !values.PlainDecimal(s, 10, 10) { log(s) }; r, _ := new(big.Rat).SetString(s); _ = r`, 1},
		{"not negated", `if values.PlainDecimal(s, 10, 10) { return }; r, _ := new(big.Rat).SetString(s); _ = r`, 1},
		{"an and another operand short-circuits", `if s != "" && !values.PlainDecimal(s, 10, 10) { return }; r, _ := new(big.Rat).SetString(s); _ = r`, 1},
		{"fence nested in a conditional block", `if ok { if !values.PlainDecimal(s, 10, 10) { return } }; r, _ := new(big.Rat).SetString(s); _ = r`, 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			src := "package p\nfunc parse(s string) {\n" + tc.body + "\n}\n"
			fset := token.NewFileSet()
			file, err := parser.ParseFile(fset, "p.go", src, parser.SkipObjectResolution)
			if err != nil {
				t.Fatal(err)
			}
			if found, _ := unfencedRatParses(fset, "p.go", file); len(found) != tc.want {
				t.Fatalf("found %d unfenced parses, want %d: %v", len(found), tc.want, found)
			}
		})
	}
}
