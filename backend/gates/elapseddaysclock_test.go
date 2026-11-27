// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind census H2

package gates

// A day count from an activity's time reads that time's precision. A day entry
// is stored at noon UTC, which far east of UTC is already the next day.
// occurredday.DaysSince counts it from its own date; elapsed.Days does not.
//
// The census is every function that calls elapsed.Days. Each one says its
// start is not an activity's time, or is listed as still counting without it.

import (
	"go/ast"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/gatekit"
)

// notAnActivityTime: elapsed.Days callers whose start is never an activity's
// time, keyed by `path:function`.
var notAnActivityTime = gatekit.Waive(map[string]string{
	"internal/shared/kernel/occurredday/occurredday.go:DaysSince":   "the precision-aware count itself; it hands elapsed.Days the noon of the date it read",
	"internal/compose/forecastcallbaseline.go:callWindow":           "a forecast period's bounds, not an activity time",
	"internal/compose/accountcoverage.go:silentDays":                "a won deal's closed_at; the exchange arm counts through contactDays with its precision",
	"internal/compose/contact360/momentpromise.go:overdueClaimCard": "a promise's due instant, counted to today",
	"internal/compose/meetingbrief/ranking.go:urgency":              "a claim's due instant, counted from today",
	"internal/compose/dealstatus/move.go:until":                     "a meeting start already moved to its date's start by meetingDay",
	"internal/shared/kernel/owedwork/owedwork.go:LateWords":         "a task's due instant",
	"internal/shared/kernel/owedwork/duecopy.go:UpcomingWords":      "a task's due instant",
	"internal/shared/kernel/deadline/deadline.go:WallDayOffset":     "a deadline's wall day in the reader's zone",
	"internal/modules/assurance/risk.go:stuckDays":                  "when the deal entered its stage, the history row's own instant",
	"internal/modules/assurance/risk.go:closeDaysFrom":              "a close date anchored in the reader's zone",
	"internal/modules/agents/tools_commitments.go:daysOverdue":      "a commitment's due instant",
	"internal/modules/forecasting/focus.go:closesInDays":            "a close date, not an activity time",
})

// storedClockStillCounted: callers that count from a stored activity clock
// without its precision yet, each with the reader it serves.
var storedClockStillCounted = gatekit.Waive(map[string]string{})

// elapsedDaysCallerCount is how many functions call elapsed.Days today.
const elapsedDaysCallerCount = 13

func TestEveryDayCountFromATimeSaysWhetherItIsAnActivitysTime(t *testing.T) {
	t.Parallel()
	defer notAnActivityTime.AssertAllMatched(t)
	defer storedClockStillCounted.AssertAllMatched(t)
	files := gatekit.Scope{
		Roots: []string{"internal"},
		Subject: func(path string, file *ast.File) bool {
			return !strings.HasSuffix(path, "_test.go") && len(elapsedDaysCallers(file)) > 0
		},
	}.Files(t)
	found := 0
	for _, file := range files {
		for _, name := range elapsedDaysCallers(file.File) {
			found++
			key := file.Path + ":" + name
			if notAnActivityTime.Waived(t, key) || storedClockStillCounted.Waived(t, key) {
				continue
			}
			t.Errorf("%s counts days with elapsed.Days. If its start is an activity's time, count it with "+
				"occurredday.DaysSince and the precision of that activity, so a day entry counts from its own "+
				"date; otherwise say in notAnActivityTime what its start is.", key)
		}
	}
	// The exact count. A walk that stopped finding the call fails instead of
	// reading green. A new caller is met here too.
	if found != elapsedDaysCallerCount {
		t.Errorf("found %d function(s) calling elapsed.Days, want exactly %d: the walk lost one, or a caller "+
			"was added or removed; update elapsedDaysCallerCount once the lists above place it", found, elapsedDaysCallerCount)
	}
}

// elapsedDaysCallers names each declaration of file that calls elapsed.Days.
func elapsedDaysCallers(file *ast.File) []string {
	var names []string
	for _, decl := range file.Decls {
		name := "package"
		if fn, isFunc := decl.(*ast.FuncDecl); isFunc {
			name = fn.Name.Name
		}
		calls := false
		ast.Inspect(decl, func(n ast.Node) bool {
			call, isCall := n.(*ast.CallExpr)
			if !isCall {
				return !calls
			}
			selector, isSelector := call.Fun.(*ast.SelectorExpr)
			if !isSelector || selector.Sel.Name != "Days" {
				return true
			}
			if pkg, isIdent := selector.X.(*ast.Ident); isIdent && pkg.Name == "elapsed" {
				calls = true
			}
			return !calls
		})
		if calls {
			names = append(names, name)
		}
	}
	return names
}
