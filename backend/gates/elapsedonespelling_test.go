// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// Every duration-to-day division is reviewed, including sites outside display
// packages. Exceptions name a function so another reader in its file is gated.
func TestOnlyElapsedCountsDaysOfSilence(t *testing.T) {
	t.Parallel()
	waivers := gatekit.Waive(map[string]string{
		"internal/shared/kernel/elapsed/elapsed.go:Days":                          "The calendar owner subtracts dates anchored at UTC midnight, so DST cannot shorten a day.",
		"internal/shared/kernel/elapsed/elapsed.go:FullDaysUntil":                 "Remaining time promises complete periods, so a midnight boundary cannot consume a deadline day.",
		"internal/compose/briefs/briefscore.go:briefTimingScore":                  "Both operands are date columns anchored at UTC midnight; today was resolved in the installation zone before ranking.",
		"internal/modules/activities/waitinghorizon.go:derivedWaitingHorizonDays": "The horizon is learned from elapsed reply latencies; calendar rounding would distort the measured distribution.",
		"internal/modules/activities/scheduledsend.go:validateSchedule":           "The label names a fixed scheduling duration limit, not an activity age or a date difference.",
		"internal/modules/consent/reviewcontext.go:refuseAnExchangeTooOldToCount": "The refusal names a configured duration window; it does not count dates since the exchange.",
		"internal/modules/deals/stageprogressionreport.go:observationDays":        "Qualification requires complete observation periods; crossing midnight must not satisfy the minimum evidence window early.",
		"internal/modules/deals/health.go:daysBetween":                            "Fractional elapsed days feed the health decay curve; rounding to calendar boundaries would change the score discontinuously.",
		"internal/modules/contacts/leadscore.go:ScoreLeadDetail":                  "Fractional elapsed days feed behavioral decay; a local midnight cannot change the age of a signal abruptly.",
		"internal/modules/search/graphtrust.go:rankScore":                         "Search freshness is continuous exponential decay rather than a displayed calendar age.",
		"internal/shared/kernel/relstrength/relstrength.go:Compute":               "Relationship strength decays with elapsed time, so a calendar boundary cannot change its score abruptly.",
	})
	defer waivers.AssertAllMatched(t)
	files := (gatekit.Scope{
		Roots:   []string{"internal"},
		Subject: func(_ string, file *ast.File) bool { return len(durationDayFunctions(file)) > 0 },
	}).Files(t)
	if len(files) < 10 {
		t.Fatalf("duration census found only %d source files", len(files))
	}
	for _, file := range files {
		for _, name := range durationDayFunctions(file.File) {
			key := file.Path + ":" + name
			if !waivers.Waived(t, key) {
				t.Errorf("%s divides elapsed hours into days; use elapsed.Days with the record zone or justify this duration", key)
			}
		}
	}
}

func durationDayFunctions(file *ast.File) []string {
	var found []string
	for _, decl := range file.Decls {
		name := "package"
		if fn, ok := decl.(*ast.FuncDecl); ok {
			name = fn.Name.Name
		}
		ast.Inspect(decl, func(node ast.Node) bool {
			division, ok := node.(*ast.BinaryExpr)
			if !ok || division.Op != token.QUO || !dayDivisor(division.Y) {
				return true
			}
			call, ok := ast.Unparen(division.X).(*ast.CallExpr)
			if !ok || len(call.Args) != 0 {
				return true
			}
			method, ok := call.Fun.(*ast.SelectorExpr)
			if ok && method.Sel.Name == "Hours" {
				found = append(found, name)
			}
			return true
		})
	}
	return found
}

func dayDivisor(expr ast.Expr) bool {
	if paren, ok := expr.(*ast.ParenExpr); ok {
		return dayDivisor(paren.X)
	}
	if name, ok := expr.(*ast.Ident); ok {
		return name.Name == "hoursPerDay"
	}
	literal, ok := expr.(*ast.BasicLit)
	if !ok || (literal.Kind != token.INT && literal.Kind != token.FLOAT) {
		return false
	}
	if literal.Kind == token.INT {
		value, err := strconv.ParseInt(literal.Value, 0, 64)
		return err == nil && value == 24
	}
	value, err := strconv.ParseFloat(literal.Value, 64)
	return err == nil && value == 24
}

func TestTheGateCanStillSeeItsOwnSubject(t *testing.T) {
	t.Parallel()
	for _, body := range []string{
		"d.Hours()/24", "(d.Hours()) / 0x18", "d.Hours() / 2_4", "d.Hours() / hoursPerDay", "d.Hours() / (24)",
		"d.Hours() / 2.4e1", "d.Hours() /\n24", "func() float64 { return d.Hours()/24 }()",
	} {
		file, err := parser.ParseFile(token.NewFileSet(), "seed.go", "package p\nfunc age() { _ = "+body+" }", 0)
		if err != nil {
			t.Fatal(err)
		}
		if len(durationDayFunctions(file)) != 1 {
			t.Errorf("missed %s", body)
		}
	}
	initializer, err := parser.ParseFile(token.NewFileSet(), "seed.go", "package p\nvar age = d.Hours()/24", 0)
	if err != nil {
		t.Fatal(err)
	}
	if got := durationDayFunctions(initializer); len(got) != 1 || got[0] != "package" {
		t.Fatalf("missed package initializer: %v", got)
	}
	file, err := parser.ParseFile(token.NewFileSet(), "seed.go", "package p\n// d.Hours()/24\nfunc age() { _ = d.Hours()/12 }", 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(durationDayFunctions(file)) != 0 {
		t.Error("flagged a comment or an hourly conversion")
	}
}
