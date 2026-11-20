// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion
//gate:kind prohibition H2

package gates

// A certification pass rate has one interval formula and is printed only with it.
//
// The verdict grades on wilsonBounds in aicert/stats.go. A second binomial
// formula could print an interval the verdict never read. A reliability printed
// as a bare point invites quoting a difference inside the sampling noise.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"path/filepath"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

const (
	aicertDir        = "internal/compose/aicert"
	wilsonOwnerFile  = aicertDir + "/stats.go"
	wilsonOwnerFunc  = "wilsonBounds"
	reliabilityField = "Reliability"
	reliabilityCell  = "ReliabilityCell"
)

// printingCalls are the calls that put text in front of a reader. Errorf and
// Fatalf are left out: a failing test's message is a diagnosis, not a report.
var printingCalls = map[string]bool{
	"Sprintf": true, "Printf": true, "Fprintf": true, "Sprint": true, "Print": true, "Fprint": true,
	"Sprintln": true, "Println": true, "Fprintln": true, "Logf": true, "Log": true,
	"FormatFloat": true, "WriteString": true,
}

func TestOnePassRateIntervalFormula(t *testing.T) {
	t.Parallel()
	owners := 0
	eachGoFileUnder(t, []string{".", filepath.Join(repoRoot, "extensions")}, func(path string, file *ast.File) {
		for _, fn := range binomialVarianceFuncs(file) {
			if path == wilsonOwnerFile && fn == wilsonOwnerFunc {
				owners++
				continue
			}
			t.Errorf("%s: %s computes a binomial interval of its own; call aicert.ReliabilityBounds, "+
				"which reads the verdict's %s", path, fn, wilsonOwnerFunc)
		}
	})
	if owners != 1 {
		t.Fatalf("found %s in %s %d times, want 1: the scan no longer recognises the formula it guards",
			wilsonOwnerFunc, wilsonOwnerFile, owners)
	}
}

func TestCertificationReliabilityPrintsWithItsInterval(t *testing.T) {
	t.Parallel()
	cellCalls := 0
	eachGoFileUnder(t, []string{aicertDir}, func(path string, file *ast.File) {
		bare, cells := bareReliabilityPrints(file)
		cellCalls += cells
		for _, pos := range bare {
			t.Errorf("%s: prints a reliability without its interval; print aicert.ReliabilityCell(passed, runs)",
				gatekit.SourceFileSet().Position(pos))
		}
	})
	if cellCalls == 0 {
		t.Fatalf("no call to %s under %s: the scan read nothing", reliabilityCell, aicertDir)
	}
}

// TestTheIntervalGatesSeeEachShape holds both scans to the forms they must
// catch and the forms they must pass.
func TestTheIntervalGatesSeeEachShape(t *testing.T) {
	t.Parallel()
	formulas := map[string]struct {
		body string
		want int
	}{
		"wald":             {`p := 0.5; _ = math.Sqrt(p * (1 - p) / n)`, 1},
		"reversed float":   {`p := 0.5; _ = (1.0 - p) * p`, 1},
		"other operands":   {`p, q := 0.5, 0.2; _ = p * (1 - q)`, 0},
		"named complement": {`p := 0.5; q := 1 - p; _ = p * q`, 0},
	}
	for name, c := range formulas {
		if got := len(binomialVarianceFuncs(parseGateSnippet(t, name, c.body))); got != c.want {
			t.Errorf("%s: found %d binomial formula(s), want %d", name, got, c.want)
		}
	}
	prints := map[string]struct {
		body       string
		bare, cell int
	}{
		"field point":   {`fmt.Sprintf("%.2f", rec.Reliability)`, 1, 0},
		"method point":  {`t.Logf("%v", tally.Reliability())`, 1, 0},
		"float point":   {`strconv.FormatFloat(*b.Reliability, 'f', 2, 64)`, 1, 0},
		"with interval": {`fmt.Sprintf("%s", aicert.ReliabilityCell(r.Passed, r.Runs))`, 0, 1},
		"diagnosis":     {`t.Errorf("reliability=%v", rec.Reliability)`, 0, 0},
	}
	for name, c := range prints {
		bare, cells := bareReliabilityPrints(parseGateSnippet(t, name, c.body))
		if len(bare) != c.bare || cells != c.cell {
			t.Errorf("%s: found %d bare print(s) and %d cell call(s), want %d and %d", name, len(bare), cells, c.bare, c.cell)
		}
	}
}

func parseGateSnippet(t *testing.T, name, body string) *ast.File {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), name+".go", "package p\nfunc f() {\n"+body+"\n}\n", 0)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return file
}

// binomialVarianceFuncs names each function holding p·(1−p), the variance every
// interval on a pass rate starts from. A complement held in its own variable
// escapes it, and the snippet test pins that limit.
func binomialVarianceFuncs(file *ast.File) []string {
	var names []string
	for _, decl := range file.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Body == nil {
			continue
		}
		found := false
		ast.Inspect(fn.Body, func(n ast.Node) bool {
			mul, ok := n.(*ast.BinaryExpr)
			if ok && mul.Op == token.MUL && (isOneMinus(mul.X, mul.Y) || isOneMinus(mul.Y, mul.X)) {
				found = true
			}
			return !found
		})
		if found {
			names = append(names, fn.Name.Name)
		}
	}
	return names
}

// isOneMinus reports whether expr is 1 − of, parenthesised or not.
func isOneMinus(expr, of ast.Expr) bool {
	for {
		paren, ok := expr.(*ast.ParenExpr)
		if !ok {
			break
		}
		expr = paren.X
	}
	sub, ok := expr.(*ast.BinaryExpr)
	if !ok || sub.Op != token.SUB {
		return false
	}
	one, ok := sub.X.(*ast.BasicLit)
	if !ok || (one.Value != "1" && one.Value != "1.0") {
		return false
	}
	return types.ExprString(sub.Y) == types.ExprString(of)
}

// bareReliabilityPrints answers the printing calls that read a Reliability field
// or method, and how many calls to ReliabilityCell the file makes.
func bareReliabilityPrints(file *ast.File) (bare []token.Pos, cells int) {
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		name := calledName(call)
		if name == reliabilityCell {
			cells++
		}
		if printingCalls[name] && readsReliability(call.Args) {
			bare = append(bare, call.Pos())
		}
		return true
	})
	return bare, cells
}

// readsReliability reports whether an argument reads .Reliability directly. A
// call to ReliabilityCell is not descended into: its output is the interval.
func readsReliability(args []ast.Expr) bool {
	found := false
	for _, arg := range args {
		ast.Inspect(arg, func(n ast.Node) bool {
			if call, ok := n.(*ast.CallExpr); ok && calledName(call) == reliabilityCell {
				return false
			}
			if sel, ok := n.(*ast.SelectorExpr); ok && sel.Sel.Name == reliabilityField {
				found = true
			}
			return !found
		})
	}
	return found
}
