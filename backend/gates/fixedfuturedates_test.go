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
//   - A day in a shape other than 2030-01-02, 2 Jan 2030 or 20300102T100000,
//     such as 01/02/2030 or a bare 20300102.
//   - Whether a waived day still flows only where its reason says. A reason is
//     checked for being stated, not for being true.

import (
	"fmt"
	"go/ast"
	"go/build/constraint"
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

// rootCount is what the census read under one root of the tree.
type rootCount struct{ files, days int }

// censusFloors sit a little below the counts measured under each root. A floor
// per root catches a walk that lost one subtree while the total still clears.
var censusFloors = []struct {
	root  string
	floor rootCount
}{
	{"internal/compose", rootCount{files: 2600, days: 1500}},
	{"internal/modules", rootCount{files: 900, days: 330}},
}

// fixedDate is one calendar day spelled as a literal in a test file.
type fixedDate struct {
	day  time.Time
	line int
}

// dayShapes are the spellings of a day a string literal can carry. They are a
// date or RFC 3339 instant, a mail Date header, and an iCal basic instant.
var dayShapes = []struct {
	pattern *regexp.Regexp
	layouts []string
}{
	{regexp.MustCompile(`[0-9]{4}-[0-9]{2}-[0-9]{2}`), []string{time.DateOnly}},
	{
		regexp.MustCompile(`[0-9]{1,2} (?:Jan|Feb|Mar|Apr|May|Jun|Jul|Aug|Sep|Oct|Nov|Dec) [0-9]{4}`),
		[]string{"2 Jan 2006", "02 Jan 2006"},
	},
	{regexp.MustCompile(`[0-9]{8}T[0-9]{6}`), []string{"20060102T150405"}},
}

func TestNoIntegrationTestSendsAFixedFutureDay(t *testing.T) {
	t.Parallel()
	defer fixedFutureDayWaivers.AssertAllMatched(t)

	counted := map[string]rootCount{}
	var findings []string
	// TestMain has left the working directory at the module root.
	err := gatekit.WalkDir(".", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") {
			return err
		}
		testOnly, err := builtOnlyForAnIntegrationLane(path)
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
		spelled := fixedDates(gatekit.SourceFileSet(), file)
		rel := filepath.ToSlash(path)
		root := censusRoot(rel)
		counted[root] = rootCount{files: counted[root].files + 1, days: counted[root].days + len(spelled)}
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
	if short := censusShortfalls(counted); len(short) > 0 {
		t.Fatalf("the census went short, and a smaller census would report PASS: %s",
			strings.Join(short, "; "))
	}
	sort.Strings(findings)
	for _, finding := range findings {
		t.Errorf("%s is a fixed day on or after %s in an integration test. Once it passes, a writer "+
			"that compares it with today refuses it. Date it from clocktest.Now(t); if the test pins "+
			"the product's clock or no writer compares the day, waive %q in fixedFutureDayWaivers and "+
			"say why. If the day is already past, move fixedDateHorizon forward instead of waiving it.",
			finding, fixedDateHorizon.Format(time.DateOnly), fixedDayWaiverKeyOf(finding))
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
		{"a mail Date header", `_ = "Date: Tue, 10 Nov 2026 09:00:00 +0000"`, nil},
		{"a future mail Date header", `_ = "Date: Fri, 5 Mar 2027 09:00:00 +0000"`, []string{"2027-03-05"}},
		{"an iCal basic instant", `_ = "DTSTART:20261202T100000Z"`, []string{"2026-12-02"}},
		{"a bare eight-digit day", `_ = "20261202"`, nil},
	}
	for _, planted := range cases {
		src := "package x\n\nimport \"time\"\n\nfunc f() {\n" + planted.body + "\n}\n"
		if got := plantedFutureDays(t, src); !equalStrings(got, planted.want) {
			t.Errorf("%s: want %q, got %q", planted.name, planted.want, got)
		}
	}
	for name, src := range map[string]string{
		"an aliased time import": "package x\n\nimport walltime \"time\"\n\nvar d = walltime.Date(2030, walltime.March, 4, 0, 0, 0, 0, walltime.UTC)\n",
		"a dot-imported time":    "package x\n\nimport . \"time\"\n\nvar d = Date(2030, March, 4, 0, 0, 0, 0, UTC)\n",
	} {
		if got := plantedFutureDays(t, src); !equalStrings(got, []string{"2030-03-04"}) {
			t.Errorf("%s: want [2030-03-04], got %q", name, got)
		}
	}
}

// TestACensusThatLostARootFails drops each root in turn. A floor on the total
// alone would pass a walk that no longer reaches internal/modules.
func TestACensusThatLostARootFails(t *testing.T) {
	t.Parallel()

	full := map[string]rootCount{}
	for _, root := range censusFloors {
		full[root.root] = root.floor
	}
	if short := censusShortfalls(full); len(short) != 0 {
		t.Fatalf("a census at every floor: want no shortfall, got %q", short)
	}
	for _, dropped := range censusFloors {
		partial := map[string]rootCount{"elsewhere": {files: 10_000, days: 10_000}}
		for _, root := range censusFloors {
			if root.root != dropped.root {
				partial[root.root] = root.floor
			}
		}
		if short := censusShortfalls(partial); len(short) != 1 || !strings.Contains(short[0], dropped.root) {
			t.Errorf("a census without %s: want one shortfall naming it, got %q", dropped.root, short)
		}
	}
}

// Every lane that builds only with the integration tag is in the corpus, the
// bench and rolesweep lanes too. A file any untagged build links is not.
func TestEveryIntegrationLaneIsInTheCorpus(t *testing.T) {
	t.Parallel()

	for line, want := range map[string]bool{
		"//go:build integration":              true,
		"//go:build integration && bench":     true,
		"//go:build integration && rolesweep": true,
		"//go:build integration && !race":     true,
		"//go:build !integration":             false,
		"//go:build integration || !windows":  false,
		"//go:build livesmoke":                false,
	} {
		expr, err := constraint.Parse(line)
		if err != nil {
			t.Fatal(err)
		}
		if got := integrationLaneOnly(expr); got != want {
			t.Errorf("%q: in the corpus = %v, want %v", line, got, want)
		}
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

// censusRoot is the root a file counts under: internal/compose and
// internal/modules each by name, and every other path together.
func censusRoot(path string) string {
	for _, root := range censusFloors {
		if strings.HasPrefix(path, root.root+"/") {
			return root.root
		}
	}
	return "elsewhere"
}

// censusShortfalls names each root read below its floor.
func censusShortfalls(counted map[string]rootCount) []string {
	var short []string
	for _, root := range censusFloors {
		got := counted[root.root]
		if got.files < root.floor.files || got.days < root.floor.days {
			short = append(short, fmt.Sprintf("%s read %d file(s) and %d day(s), under its floors of %d and %d",
				root.root, got.files, got.days, root.floor.files, root.floor.days))
		}
	}
	return short
}

// builtOnlyForAnIntegrationLane reports whether only an integration lane builds
// the file: plain integration, or integration with a lane tag such as bench.
func builtOnlyForAnIntegrationLane(path string) (bool, error) {
	src, err := gatekit.ReadFile(path)
	if err != nil {
		return false, err
	}
	expr, err := buildConstraint(string(src))
	if expr == nil || err != nil {
		return false, err
	}
	return integrationLaneOnly(expr), nil
}

// integrationLaneOnly evaluates the constraint with every tag it asks for set.
// It admits the file when that build includes it, integration is among those
// tags, and a build with no tags leaves it out.
func integrationLaneOnly(expr constraint.Expr) bool {
	asked := map[string]bool{}
	askedTags(expr, true, asked)
	return asked["integration"] && expr.Eval(func(tag string) bool { return asked[tag] }) &&
		!expr.Eval(func(string) bool { return false })
}

// askedTags collects the tags the constraint wants set, those under an even
// number of negations.
func askedTags(expr constraint.Expr, wanted bool, into map[string]bool) {
	switch node := expr.(type) {
	case *constraint.TagExpr:
		if wanted {
			into[node.Tag] = true
		}
	case *constraint.NotExpr:
		askedTags(node.X, !wanted, into)
	case *constraint.AndExpr:
		askedTags(node.X, wanted, into)
		askedTags(node.Y, wanted, into)
	case *constraint.OrExpr:
		askedTags(node.X, wanted, into)
		askedTags(node.Y, wanted, into)
	}
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
	names, dotted := timePackageNames(file)
	timeName := func(expr ast.Expr, name string) bool {
		if ident, ok := expr.(*ast.Ident); ok {
			return dotted && ident.Name == name
		}
		selector, ok := expr.(*ast.SelectorExpr)
		if !ok || selector.Sel.Name != name {
			return false
		}
		pkg, ok := selector.X.(*ast.Ident)
		return ok && names[pkg.Name]
	}
	var found []fixedDate
	ast.Inspect(file, func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CallExpr:
			if day, ok := timeDateLiteral(node, timeName); ok {
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

// daysInString reads each day in a dayShapes spelling that no further digit
// runs into, so 12030-03-045 is no day while 2030-03-04/2030-03-05 is two.
func daysInString(text string) []time.Time {
	var days []time.Time
	for _, shape := range dayShapes {
		for _, span := range shape.pattern.FindAllStringIndex(text, -1) {
			if span[0] > 0 && isDigit(text[span[0]-1]) || span[1] < len(text) && isDigit(text[span[1]]) {
				continue
			}
			for _, layout := range shape.layouts {
				if at, err := time.Parse(layout, text[span[0]:span[1]]); err == nil {
					days = append(days, time.Date(at.Year(), at.Month(), at.Day(), 0, 0, 0, 0, time.UTC))
					break
				}
			}
		}
	}
	return days
}

// timeDateLiteral reads time.Date(year, month, day, ...) with a literal year.
// A month or day it cannot read is taken as the last one. An unreadable part
// then makes the day later and never hides it.
func timeDateLiteral(call *ast.CallExpr, timeName func(ast.Expr, string) bool) (time.Time, bool) {
	if len(call.Args) < 3 || !timeName(call.Fun, "Date") {
		return time.Time{}, false
	}
	year, ok := intLiteral(call.Args[0])
	if !ok {
		return time.Time{}, false
	}
	month, ok := monthLiteral(call.Args[1], timeName)
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
func monthLiteral(expr ast.Expr, timeName func(ast.Expr, string) bool) (time.Month, bool) {
	if n, ok := intLiteral(expr); ok {
		return time.Month(n), n >= 1 && n <= 12
	}
	for month := time.January; month <= time.December; month++ {
		if timeName(expr, month.String()) {
			return month, true
		}
	}
	return 0, false
}
