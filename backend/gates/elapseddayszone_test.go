// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind claim H1

package gates

// A day count is reckoned in the zone the sentence is drawn in.
//
// elapsed.Days takes the zone as its third argument so a caller cannot forget
// it. This gate fails the two ways a caller can still count in UTC: passing
// time.UTC, time.Local or nil as the zone, and flooring to a UTC day by hand
// with Truncate(24 * time.Hour) in a file that counts days.
//
// The subject is derived from the tree: every non-test Go file under
// internal/ that imports shared/kernel/elapsed. The UTC day floors that stay
// (rate effective dates, rollups, automation clocks) are dates of rates and
// rollups, not reader sentences, and none of those files imports elapsed.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const elapsedImport = `"github.com/margince/margince/backend/internal/shared/kernel/elapsed"`

// minElapsedCallers is the floor under the census: fewer files than this means
// the walk is reading a smaller tree than the one that has callers.
const minElapsedCallers = 15

func TestEveryDayCountNamesAZone(t *testing.T) {
	t.Parallel()
	files := elapsedCallerFiles(t)
	if len(files) < minElapsedCallers {
		t.Fatalf("only %d files import elapsed, want at least %d: the walk reads a smaller tree than it should", len(files), minElapsedCallers)
	}
	for _, path := range files {
		for _, problem := range zonelessDayCounts(t, path, nil) {
			t.Error(problem)
		}
	}
}

func TestTheZoneGateSeesEachWayToCountInUTC(t *testing.T) {
	t.Parallel()
	cases := map[string]string{
		"nil zone":          `_ = elapsed.Days(a, b, nil)`,
		"time.UTC":          `_ = elapsed.Days(a, b, time.UTC)`,
		"time.Local":        `_ = elapsed.Days(a, b, time.Local)`,
		"hand floor":        `_ = a.UTC().Truncate(24 * time.Hour)`,
		"named hours floor": `_ = a.UTC().Truncate(hoursPerDay * time.Hour)`,
	}
	for name, body := range cases {
		src := "package p\nimport (\n\"time\"\n" + elapsedImport + "\n)\nfunc f(a, b time.Time) {\n" + body + "\n}\n"
		if got := zonelessDayCounts(t, "seeded.go", []byte(src)); len(got) == 0 {
			t.Errorf("%s: the gate did not flag %q", name, body)
		}
	}
	clean := "package p\nimport (\n\"time\"\n" + elapsedImport + "\n)\nfunc f(a, b time.Time, loc *time.Location) {\n_ = elapsed.Days(a, b, loc)\n}\n"
	if got := zonelessDayCounts(t, "clean.go", []byte(clean)); len(got) != 0 {
		t.Errorf("a zoned call was flagged: %v", got)
	}
}

// elapsedCallerFiles lists the non-test, non-generated Go files that import elapsed.
func elapsedCallerFiles(t *testing.T) []string {
	t.Helper()
	var out []string
	err := filepath.WalkDir("internal", func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") ||
			strings.HasSuffix(path, "_test.go") || strings.HasSuffix(path, "_gen.go") {
			return nil
		}
		raw, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		if strings.Contains(string(raw), elapsedImport) && !strings.HasPrefix(path, filepath.Join("internal", "shared", "kernel", "elapsed")) {
			out = append(out, path)
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking internal: %v", err)
	}
	return out
}

// zonelessDayCounts parses one file (or src, when given) and names every
// elapsed.Days call with no real zone and every hand-written UTC day floor.
func zonelessDayCounts(t *testing.T, path string, src []byte) []string {
	t.Helper()
	fset := token.NewFileSet()
	var source any
	if src != nil {
		source = src
	}
	file, err := parser.ParseFile(fset, path, source, 0)
	if err != nil {
		t.Fatalf("parsing %s: %v", path, err)
	}
	var out []string
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok {
			return true
		}
		sel, ok := call.Fun.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		where := fset.Position(call.Pos()).String()
		switch {
		case isIdent(sel.X, "elapsed") && sel.Sel.Name == "Days":
			if len(call.Args) != 3 || isUTCZone(call.Args[2]) {
				out = append(out, where+": elapsed.Days needs the zone the sentence is drawn in as its third argument, not nil, time.UTC or time.Local")
			}
		case sel.Sel.Name == "Truncate" && len(call.Args) == 1 && isDayDuration(call.Args[0]):
			out = append(out, where+": Truncate by a day floors to a UTC day; use elapsed.CalendarDay with the reader's zone")
		}
		return true
	})
	return out
}

func isIdent(e ast.Expr, name string) bool {
	id, ok := e.(*ast.Ident)
	return ok && id.Name == name
}

func isUTCZone(e ast.Expr) bool {
	if isIdent(e, "nil") {
		return true
	}
	sel, ok := e.(*ast.SelectorExpr)
	return ok && isIdent(sel.X, "time") && (sel.Sel.Name == "UTC" || sel.Sel.Name == "Local")
}

// isDayDuration matches `24 * time.Hour` and `hoursPerDay * time.Hour`.
func isDayDuration(e ast.Expr) bool {
	mul, ok := e.(*ast.BinaryExpr)
	if !ok || mul.Op != token.MUL {
		return false
	}
	if lit, ok := mul.X.(*ast.BasicLit); ok && lit.Value == "24" {
		return true
	}
	return isIdent(mul.X, "hoursPerDay")
}
