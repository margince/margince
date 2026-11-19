// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// An integration test spells no calendar day after fixedDateHorizon.
//
// A deal writer refuses a close date in the past, and an expiry check compares
// with the database's clock. A test that sends such a writer a fixed "future"
// day passes until that day comes, then fails on a day nobody edited anything.
// The drift lane cannot find it: it moves clocktest.Now, and a literal does not
// move. So the date comes from clocktest.Now(t), or the test pins the product's
// clock and carries a waiver that says so.
//
// wallclockfixtures_test.go owns the other half, the fixtures dated from
// time.Now. This gate owns the literal day.
//
// The horizon is a constant, not today. Measured against today, a waiver would
// stop matching once its day passed. The stale-waiver check would then fail
// main on a day nobody edited anything. Move the horizon forward by an edit;
// the waivers it passes then go stale and come out.
//
// What this does not cover, so its silence is not read as a wider claim:
//   - A unit test. Its writer takes an injected clock, and a fixed day there is
//     compared with the day the test injects.
//   - A year held in a named constant, or a date built from parts the parser
//     cannot read as literals.
//   - A day written in another shape, such as 01/02/2030 or 20300102.
//   - Whether a waived day still flows only where its reason says. A reason is
//     checked for being stated, not for being true.

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// fixedDateHorizon is the first day this gate treats as future.
var fixedDateHorizon = time.Date(2026, time.November, 19, 0, 0, 0, 0, time.UTC)

// neverYear starts the days a test writes to mean "never expires". A suite
// that still runs then has rewritten these tests many times over.
const neverYear = 2090

// The census floors sit below the real counts. They catch a walk or a parser
// that stopped seeing, not a tree that changed.
const (
	integrationFileFloor = 1000
	fixedDateFloor       = 1000
)

// fixedDate is one calendar day spelled as a literal in a test file.
type fixedDate struct {
	day  time.Time
	line int
}

// isoDay finds a calendar day inside a string literal: a bare date, an RFC
// 3339 instant, or a value in a JSON payload.
var isoDay = regexp.MustCompile(`[0-9]{4}-[0-9]{2}-[0-9]{2}`)

func TestNoIntegrationTestSendsAFixedFutureDay(t *testing.T) {
	t.Parallel()
	defer fixedFutureDayWaivers.AssertAllMatched(t)

	files, dates := 0, 0
	var findings []string
	// TestMain has left the working directory at the module root.
	err := gatekit.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		testOnly, err := builtOnlyForIntegration(path)
		if err != nil {
			t.Errorf("reading %s's build constraint: %v", filepath.ToSlash(path), err)
			return nil
		}
		if !testOnly {
			return nil
		}
		file, err := gatekit.ParseFile(path, 0)
		if err != nil {
			t.Errorf("parsing %s: %v", filepath.ToSlash(path), err)
			return nil
		}
		files++
		spelled := fixedDates(gatekit.SourceFileSet(), file)
		dates += len(spelled)
		rel := filepath.ToSlash(path)
		for _, date := range futureDays(spelled, fixedDateHorizon) {
			if !fixedFutureDayWaivers.Waived(t, fixedDayWaiverKey(rel, date.day)) {
				findings = append(findings, fmt.Sprintf("%s:%d: %s", rel, date.line, date.day.Format(time.DateOnly)))
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walking the module: %v", err)
	}
	if files < integrationFileFloor || dates < fixedDateFloor {
		t.Fatalf("read %d integration-only file(s) holding %d fixed day(s), below the floors of %d and %d. "+
			"The walk or the parser stopped seeing; a smaller census would report PASS.",
			files, dates, integrationFileFloor, fixedDateFloor)
	}
	sort.Strings(findings)
	for _, finding := range findings {
		t.Errorf("%s is a fixed day on or after %s in an integration test. Once it passes, a writer "+
			"that compares it with today refuses it. Date it from clocktest.Now(t); if the test pins "+
			"the product's clock or no writer compares the day, waive %q in fixedFutureDayWaivers and "+
			"say why.", finding, fixedDateHorizon.Format(time.DateOnly), fixedDayWaiverKeyOf(finding))
	}
}

// TestEveryShapeOfAFixedFutureDayIsSeen plants each way a day reaches a writer.
// A scanner that stops seeing one fails here instead of passing the tree.
func TestEveryShapeOfAFixedFutureDayIsSeen(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		body string
		want []string
	}{
		{"a typed struct field", `_ = Deal{ExpectedCloseDate: time.Date(2030, time.March, 4, 0, 0, 0, 0, time.UTC)}`, []string{"2030-03-04"}},
		{"an HTTP map key", `_ = map[string]any{"expected_close_date": "2030-03-04"}`, []string{"2030-03-04"}},
		{"a staged JSON payload", "_ = []byte(`{\"deal_id\":\"x\",\"expires_at\":\"2030-03-04T09:00:00Z\"}`)", []string{"2030-03-04"}},
		{"a local built from time.Date", "closes := time.Date(2030, 3, 4, 0, 0, 0, 0, time.UTC)\n_ = closes", []string{"2030-03-04"}},
		{"two days in one string", `_ = "2030-03-04/2030-03-05"`, []string{"2030-03-04", "2030-03-05"}},
		{"a month the parser cannot read", "_ = time.Date(2030, month, 1, 0, 0, 0, 0, time.UTC)", []string{"2030-12-31"}},
		{"a day the parser cannot read", "_ = time.Date(2026, 12, day, 0, 0, 0, 0, time.UTC)", []string{"2026-12-31"}},
		{"a past day", `_ = "2025-03-04"`, nil},
		{"a never day", `_ = time.Date(2099, 12, 31, 0, 0, 0, 0, time.UTC)`, nil},
		{"a day dated from the clock", "_ = clocktest.Now(t).AddDate(0, 0, 30)", nil},
		{"a longer number", `_ = "12030-03-045"`, nil},
	}
	for _, planted := range cases {
		src := "package x\n\nimport \"time\"\n\nfunc f() {\n" + planted.body + "\n}\n"
		if got := plantedFutureDays(t, src); !equalStrings(got, planted.want) {
			t.Errorf("%s: want %q, got %q", planted.name, planted.want, got)
		}
	}
	aliased := "package x\n\nimport walltime \"time\"\n\nvar d = walltime.Date(2030, walltime.March, 4, 0, 0, 0, 0, walltime.UTC)\n"
	if got := plantedFutureDays(t, aliased); !equalStrings(got, []string{"2030-03-04"}) {
		t.Errorf("an aliased time import: want [2030-03-04], got %q", got)
	}
}

func plantedFutureDays(t *testing.T, src string) []string {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "planted_test.go", src, 0)
	if err != nil {
		t.Fatalf("parsing a planted file: %v", err)
	}
	var days []string
	for _, date := range futureDays(fixedDates(fset, file), fixedDateHorizon) {
		days = append(days, date.day.Format(time.DateOnly))
	}
	return days
}

// futureDays keeps the days on or after the horizon and before neverYear.
func futureDays(dates []fixedDate, horizon time.Time) []fixedDate {
	var future []fixedDate
	for _, date := range dates {
		if !date.day.Before(horizon) && date.day.Year() < neverYear {
			future = append(future, date)
		}
	}
	return future
}

// fixedDayWaiverKey names one day in one file. A file-wide waiver would hide the next
// day added to it.
func fixedDayWaiverKey(path string, day time.Time) string {
	return path + " " + day.Format(time.DateOnly)
}

// fixedDayWaiverKeyOf turns a finding ("path:line: day") into the key that waives it.
func fixedDayWaiverKeyOf(finding string) string {
	location, day, _ := strings.Cut(finding, ": ")
	path := location[:strings.LastIndex(location, ":")]
	return path + " " + day
}

// fixedDates returns every calendar day the file spells out: each time.Date
// with a literal year, and each YYYY-MM-DD inside a string literal.
func fixedDates(fset *token.FileSet, file *ast.File) []fixedDate {
	names, _ := timePackageNames(file)
	var found []fixedDate
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			if day, ok := timeDateLiteral(node, names); ok {
				found = append(found, fixedDate{day: day, line: fset.Position(node.Pos()).Line})
			}
		case *ast.BasicLit:
			if node.Kind == token.STRING {
				for _, day := range daysInString(node.Value) {
					found = append(found, fixedDate{day: day, line: fset.Position(node.Pos()).Line})
				}
			}
		}
		return true
	})
	return found
}

// daysInString reads each YYYY-MM-DD not run into by a further digit, so
// 12030-03-045 is no day while 2030-03-04/2030-03-05 is two.
func daysInString(text string) []time.Time {
	var days []time.Time
	for _, span := range isoDay.FindAllStringIndex(text, -1) {
		if span[0] > 0 && isDigit(text[span[0]-1]) || span[1] < len(text) && isDigit(text[span[1]]) {
			continue
		}
		if day, err := time.Parse(time.DateOnly, text[span[0]:span[1]]); err == nil {
			days = append(days, day)
		}
	}
	return days
}

// timeDateLiteral reads time.Date(year, month, day, ...) with a literal year.
// A month or day it cannot read is taken as the last one. An unreadable part
// then makes the day later and never hides it.
func timeDateLiteral(call *ast.CallExpr, names map[string]bool) (time.Time, bool) {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != "Date" || len(call.Args) < 3 {
		return time.Time{}, false
	}
	if pkg, ok := selector.X.(*ast.Ident); !ok || !names[pkg.Name] {
		return time.Time{}, false
	}
	year, ok := intLiteral(call.Args[0])
	if !ok {
		return time.Time{}, false
	}
	month, ok := monthLiteral(call.Args[1], names)
	if !ok {
		return time.Date(year, time.December, 31, 0, 0, 0, 0, time.UTC), true
	}
	day, ok := intLiteral(call.Args[2])
	if !ok {
		day = 0
		month++
	}
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC), true
}

func intLiteral(expr ast.Expr) (int, bool) {
	literal, ok := expr.(*ast.BasicLit)
	if !ok || literal.Kind != token.INT {
		return 0, false
	}
	n, err := strconv.Atoi(literal.Value)
	return n, err == nil
}

// monthLiteral reads time.March or a literal 3.
func monthLiteral(expr ast.Expr, names map[string]bool) (time.Month, bool) {
	if n, ok := intLiteral(expr); ok {
		return time.Month(n), n >= 1 && n <= 12
	}
	selector, ok := expr.(*ast.SelectorExpr)
	if !ok {
		return 0, false
	}
	if pkg, ok := selector.X.(*ast.Ident); !ok || !names[pkg.Name] {
		return 0, false
	}
	for month := time.January; month <= time.December; month++ {
		if month.String() == selector.Sel.Name {
			return month, true
		}
	}
	return 0, false
}
