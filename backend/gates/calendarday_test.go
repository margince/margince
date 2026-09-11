// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind prohibition H2

package gates

// A calendar day is one derivation, and this is the census that keeps it one.
//
// "What day is this instant?" was answered five times by `X.UTC().Truncate(24 *
// time.Hour)` — UTC's midnight wherever the installation is — and the invariant
// was re-broken three times by call-site discipline alone. storekit.WorkspaceDay
// now holds the reading (the local day of an instant), storekit.AsDate holds the
// other (the calendar date a value already names), and this gate fails the
// moment a sixth site spells the truncation itself.
//
// WHAT THIS DOES NOT COVER, so the absence of findings is not read wider than it
// is: the OTHER spelling of a day, `time.Date(t.Year(), t.Month(), t.Day(), 0,
// 0, 0, 0, ...)`, which is how storekit itself builds the value and which a
// dozen tests use to name a specific date — matching it would report far more
// correct sites than wrong ones, the noise that teaches a reader to skip a
// census. The `Truncate(24 * time.Hour)` idiom is the one every drift used, and
// the one a fourth author reaches for next; that is what this holds.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// calendarDayOwner is the package that may hold the day derivation. Its own
// spelling is time.Date, not Truncate, so it never trips this — but it is named
// so the failure can point a new site at the primitive rather than at a rule.
const calendarDayOwner = "internal/platform/database/storekit"

// deferred sites still truncate a day in UTC, each for a reason that is not "not
// yet done": converting them needs a zone the module cannot currently reach, or
// would be wrong. Ratified by name so a NEW truncation fails loudly while these
// stay described where the next reader sees them, not in a PR they will not open.
var deferred = gatekit.Waive(map[string]string{
	"internal/modules/finance/offline.go":           "the offline ledger's demonstration anchor: a fixed instant reduced to a stable date ORIGIN for reproducible synthetic invoices, never a business calendar day to localise. UTC is the whole point — a moving or zoned anchor would rewrite every generated row.",
	"internal/modules/ai/ratewrite.go":              "the ai_model_rate sheet's effective-day guard and cutoff, the fx_rate sibling. It is an operator-set business day and belongs in the installation zone, but RateStore carries no installation handle to resolve one; giving it the seam deals.Store has is its own change.",
	"internal/modules/ai/pricing.go":                "normalises the seed price sheet's effective date, the same ai_model_rate day as ratewrite.go. It moves in lockstep with that seam, not before it.",
	"internal/modules/people/rollupclock.go":        "the open-pipeline rollup's FX as-of day, deliberately UTC today so two readers of one account agree on a rate. A workspace zone meets that goal too and is the right answer, but the pure clock helper must first be threaded a zone.",
	"internal/modules/automation/handlers_clock.go": "the renewal-reminder window compares a DATE column scanned as UTC midnight against today; today is truncated the SAME way ON PURPOSE so both sides agree. Converting today alone would reintroduce the same-day miss — this moves only in lockstep with the anchor scan in candidates.go.",
})

// isDayTruncation reports whether a call is `X.Truncate(24 * time.Hour)` — the
// one idiom every calendar-day drift used, in either operand order. timeName is
// the local name the file binds the standard "time" package to, so a truncation
// written through an aliased import is still seen.
func isDayTruncation(call *ast.CallExpr, timeName string) bool {
	sel, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Truncate" || len(call.Args) != 1 {
		return false
	}
	bin, ok := unparen(call.Args[0]).(*ast.BinaryExpr)
	if !ok || bin.Op != token.MUL {
		return false
	}
	return (isIntLit(bin.X, "24") && isTimeHour(bin.Y, timeName)) ||
		(isTimeHour(bin.X, timeName) && isIntLit(bin.Y, "24"))
}

// unparen strips redundant parentheses, so `(24)` and `(time.Hour)` and a whole
// parenthesised `(24 * time.Hour)` match the same as the bare forms. A census
// that let a parenthesised operand slip would fail short (rule 8), and gofmt
// does not remove parentheses a writer put there.
func unparen(expr ast.Expr) ast.Expr {
	for {
		paren, ok := expr.(*ast.ParenExpr)
		if !ok {
			return expr
		}
		expr = paren.X
	}
}

func isIntLit(expr ast.Expr, value string) bool {
	lit, ok := unparen(expr).(*ast.BasicLit)
	return ok && lit.Kind == token.INT && lit.Value == value
}

func isTimeHour(expr ast.Expr, timeName string) bool {
	sel, ok := unparen(expr).(*ast.SelectorExpr)
	if !ok || sel.Sel.Name != "Hour" {
		return false
	}
	pkg, ok := sel.X.(*ast.Ident)
	return ok && timeName != "" && pkg.Name == timeName
}

// timeImportName is the local name this file binds the standard "time" package
// to — "time" normally, or the alias in `import clock "time"`. Empty when the
// file does not import time at all, in which case no `time.Hour` can appear.
//
// Resolving the alias is what keeps the census from failing SHORT (rule 8): a
// truncation written through an aliased import would otherwise pass unseen. A
// LOCAL value shadowing `time` is deliberately NOT resolved — it would be
// over-reported, the safe direction, an extra waiver rather than a missed site.
func timeImportName(file *ast.File) string {
	for _, imp := range file.Imports {
		if imp.Path.Value != `"time"` {
			continue
		}
		if imp.Name != nil {
			return imp.Name.Name
		}
		return "time"
	}
	return ""
}

// TestTheMatcherSeesWhatItClaimsTo pins what the idiom catches and what it must
// let through — a census asserting a shape is ABSENT passes identically over a
// clean tree and over a matcher that has quietly stopped matching.
func TestTheMatcherSeesWhatItClaimsTo(t *testing.T) {
	t.Parallel()
	caught := []string{
		"x.Truncate(24 * time.Hour)",
		"x.UTC().Truncate(24 * time.Hour)",
		"clock().UTC().Truncate(24 * time.Hour)",
		"t.Truncate(time.Hour * 24)",
		// Parentheses a writer added, which gofmt keeps: the census must see
		// through them or it fails short.
		"x.Truncate((24) * time.Hour)",
		"x.Truncate(24 * (time.Hour))",
		"x.Truncate((24 * time.Hour))",
	}
	for _, src := range caught {
		if !exprIsDayTruncation(t, src) {
			t.Errorf("the matcher does not see a day truncation it must:\n\t%s", src)
		}
	}
	missed := []string{
		"x.Truncate(time.Hour)",
		"x.Truncate(30 * time.Minute)",
		"x.Truncate(48 * time.Hour)",
		"x.Round(24 * time.Hour)",
		"time.Date(x.Year(), x.Month(), x.Day(), 0, 0, 0, 0, time.UTC)",
	}
	for _, src := range missed {
		if exprIsDayTruncation(t, src) {
			t.Errorf("the matcher reports something that is not a day truncation:\n\t%s", src)
		}
	}
}

// exprIsDayTruncation parses one expression and runs the matcher over its
// outermost call, the shape the tree walk inspects.
func exprIsDayTruncation(t *testing.T, src string) bool {
	t.Helper()
	expr, err := parser.ParseExpr(src)
	if err != nil {
		t.Fatalf("parsing %q: %v", src, err)
	}
	call, ok := expr.(*ast.CallExpr)
	return ok && isDayTruncation(call, "time")
}

// TestTheMatcherResolvesTheTimeImportAlias plants the case a static review named:
// a truncation written through an aliased `time` import is a real day derivation
// the census MUST still see (the false-NEGATIVE rule 8 forbids), while a file
// that never imports time has no truncation to find.
func TestTheMatcherResolvesTheTimeImportAlias(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		src  string
		want bool
	}{
		{"plain import", "package p\nimport \"time\"\nfunc f(x time.Time) { _ = x.Truncate(24 * time.Hour) }\n", true},
		{"aliased import", "package p\nimport clock \"time\"\nfunc f(x clock.Time) { _ = x.Truncate(24 * clock.Hour) }\n", true},
		{"no time import is nothing to find", "package p\nfunc f() { _ = 24 }\n", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := fileHasDayTruncation(t, tc.src); got != tc.want {
				t.Errorf("day truncation seen = %v, want %v in:\n%s", got, tc.want, tc.src)
			}
		})
	}
}

// fileHasDayTruncation parses a whole file and runs the census's own logic —
// resolve the time import, then inspect — so the alias resolution is exercised
// exactly as the walk exercises it.
func fileHasDayTruncation(t *testing.T, src string) bool {
	t.Helper()
	file, err := parser.ParseFile(token.NewFileSet(), "fixture.go", src, 0)
	if err != nil {
		t.Fatalf("parsing fixture: %v", err)
	}
	timeName := timeImportName(file)
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		if call, ok := node.(*ast.CallExpr); ok && isDayTruncation(call, timeName) {
			found = true
		}
		return true
	})
	return found
}

func TestOnlyOnePlaceDerivesACalendarDay(t *testing.T) {
	t.Parallel()
	// A ratification that stops matching is one for a site that moved or was
	// converted; leaving it re-exempts whatever takes its place.
	defer deferred.AssertAllMatched(t)

	var sites []string
	judged := 0
	fset := token.NewFileSet()
	for _, root := range []string{"internal", "../extensions"} {
		err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if entry.IsDir() {
				if name := entry.Name(); name == "testdata" || name == "node_modules" {
					return fs.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") ||
				strings.HasSuffix(path, "_gen.go") {
				return nil
			}
			rel := filepath.ToSlash(path)
			// Exactly the owner package, by directory prefix — Contains would also
			// exempt a sibling like storekitlegacy/ whose path merely spells the
			// owner's, letting a truncation there pass unseen.
			if rel == calendarDayOwner || strings.HasPrefix(rel, calendarDayOwner+"/") {
				return nil
			}
			source, readErr := os.ReadFile(path) // #nosec G304 -- a *.go path from walking the trusted source tree
			if readErr != nil {
				return readErr
			}
			judged++
			file, parseErr := parser.ParseFile(fset, path, source, 0)
			if parseErr != nil {
				return parseErr
			}
			timeName := timeImportName(file)
			ast.Inspect(file, func(node ast.Node) bool {
				call, ok := node.(*ast.CallExpr)
				if !ok || !isDayTruncation(call, timeName) {
					return true
				}
				if !deferred.Waived(t, rel) {
					sites = append(sites, rel)
				}
				return true
			})
			return nil
		})
		if err != nil {
			t.Fatalf("walking %s: %v", root, err)
		}
	}
	// The floor sits just under the real corpus (~2800 non-test .go files), not
	// at a token 500: under-recognition is the one way this must not break
	// (rule 8), and a walk that silently stopped reading internal/compose —
	// where org360 and briefs live — would still clear a low floor and report
	// PASS over a planted truncation. If a refactor legitimately drops the count
	// below this, lower it deliberately rather than widening the blind spot.
	if judged < 2500 {
		t.Fatalf("the census read only %d Go files, far below the ~2800 it should — it covered "+
			"a fraction of the tree and a truncation outside what it read would pass unseen", judged)
	}
	sort.Strings(sites)
	if len(sites) > 0 {
		t.Errorf("%d site(s) truncate a calendar day themselves.\n\n"+
			"Which day an instant falls on is one derivation, in the installation's zone, and it "+
			"was re-broken three times by spelling Truncate(24 * time.Hour) at a fresh call site. "+
			"Use storekit.WorkspaceDay (the local day of an instant) or storekit.AsDate (the date a "+
			"value already names).\n\n\t%s", len(sites), strings.Join(sites, "\n\t"))
	}
}
