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
	"bytes"
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"go/types"
	"os"
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

var (
	intervalScanRoots = []string{".", filepath.Join(repoRoot, "extensions")}
	// bareReliabilityWaivers are files that print a pass rate without its interval.
	bareReliabilityWaivers = gatekit.Waive(map[string]string{
		"internal/compose/mcptoolcoveragepage_test.go": "the MCP coverage rate feeds no Wilson-graded verdict, " +
			"and package compose cannot import aicert because aicert imports compose",
	})
	// knownReliabilityPrinters each print a reliability today, so a scan that
	// stops seeing them has gone blind rather than clean.
	knownReliabilityPrinters = []string{
		aicertDir + "/reportcmd/report.go",
		aicertDir + "/e2e_test.go",
		aicertDir + "/certificationpage_test.go",
		aicertDir + "/certificationfamily_test.go",
	}
	reliabilityPages = []string{
		repoRoot + "/docs/reference/ai-certification.json",
	}
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
	eachGoFileUnder(t, intervalScanRoots, func(path string, file *ast.File) {
		for _, fn := range binomialVarianceFuncs(file) {
			if path == wilsonOwnerFile && fn == wilsonOwnerFunc {
				owners++
				continue
			}
			t.Errorf("%s: %s computes a binomial interval of its own; call aicert.ReliabilityIntervalOf, "+
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
	cellsByFile := map[string]int{}
	eachGoFileUnder(t, intervalScanRoots, func(path string, file *ast.File) {
		bare, cells := bareReliabilityPrints(file)
		cellsByFile[path] += cells
		for _, pos := range unwaivedPrints(t, path, bare) {
			t.Errorf("%s: prints a reliability without its interval; print aicert.ReliabilityCell(passed, runs)",
				gatekit.SourceFileSet().Position(pos))
		}
	})
	bareReliabilityWaivers.AssertAllMatched(t)
	for _, printer := range knownReliabilityPrinters {
		if cellsByFile[printer] == 0 {
			t.Errorf("%s makes no call to %s: the scan missed a known printer, or it now prints another way",
				printer, reliabilityCell)
		}
	}
}

// TestEveryPublishedReliabilityCarriesItsInterval reads the generated JSON a
// tool consumes. Each non-null reliability there has a reliability_interval.
func TestEveryPublishedReliabilityCarriesItsInterval(t *testing.T) {
	t.Parallel()
	for _, page := range reliabilityPages {
		raw, err := os.ReadFile(page)
		if err != nil {
			t.Fatalf("reading %s: %v", page, err)
		}
		measured, missing := countReliabilityIntervals(t, page, raw)
		if measured == 0 {
			t.Errorf("%s holds no measured reliability: the scan read nothing", page)
		}
		if missing > 0 {
			t.Errorf("%s: %d of %d measured reliabilities carry no reliability_interval; regenerate it",
				page, missing, measured)
		}
	}
}

// countReliabilityIntervals walks a JSON value and counts the objects with a
// non-null reliability, and those of them without an interval.
func countReliabilityIntervals(t *testing.T, page string, raw json.RawMessage) (measured, missing int) {
	t.Helper()
	var children []json.RawMessage
	switch trimmed := bytes.TrimSpace(raw); {
	case bytes.HasPrefix(trimmed, []byte("{")):
		var object map[string]json.RawMessage
		if err := json.Unmarshal(trimmed, &object); err != nil {
			t.Fatalf("parsing %s: %v", page, err)
		}
		if isMeasured(object["reliability"]) {
			measured++
			if !isMeasured(object["reliability_interval"]) {
				missing++
			}
		}
		for _, child := range object {
			children = append(children, child)
		}
	case bytes.HasPrefix(trimmed, []byte("[")):
		if err := json.Unmarshal(trimmed, &children); err != nil {
			t.Fatalf("parsing %s: %v", page, err)
		}
	}
	for _, child := range children {
		m, x := countReliabilityIntervals(t, page, child)
		measured, missing = measured+m, missing+x
	}
	return measured, missing
}

func isMeasured(value json.RawMessage) bool {
	return len(value) > 0 && string(value) != "null"
}

// unwaivedPrints drops the bare prints of a waived file, asking the waiver only
// about a file that offends.
func unwaivedPrints(t *testing.T, path string, bare []token.Pos) []token.Pos {
	t.Helper()
	if len(bare) == 0 || bareReliabilityWaivers.Waived(t, path) {
		return nil
	}
	return bare
}

// A bare print in any other root compose file still fails: the waiver names
// one file, never its package.
func TestTheBareReliabilityWaiverCoversOneFile(t *testing.T) {
	t.Parallel()
	bare, _ := bareReliabilityPrints(parseGateSnippet(t, "planted", `fmt.Sprintf("%.2f", m.Reliability)`))
	if got := unwaivedPrints(t, "internal/compose/plantedpage_test.go", bare); len(got) != 1 {
		t.Errorf("a bare print in another compose file reports %d finding(s), want 1", len(got))
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
		"wald":           {`p := 0.5; _ = math.Sqrt(p * (1 - p) / n)`, 1},
		"reversed float": {`p := 0.5; _ = (1.0 - p) * p`, 1},
		"other operands": {`p, q := 0.5, 0.2; _ = p * (1 - q)`, 0},
		// A known blind spot, pinned so a change to it is a decision.
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
