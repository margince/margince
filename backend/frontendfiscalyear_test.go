// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package backendarch

// A fiscal year's label is spelled twice: the server builds it in SQL
// (internal/compose/reportperiod.go) because that is what a report is actually
// cut by, and the browser builds it in TypeScript
// (frontend/src/format/fiscalyear.ts) to show an admin what the setting they
// are about to save will produce.
//
// Two spellings of one rule drift, and this one drifts SILENTLY: the settings
// screen would promise "FY2026/27" while every report said something else, and
// nothing on either side would fail. An admin would discover it by disbelieving
// a report.
//
// WHAT THIS GATE CAN AND CANNOT DO, stated plainly because the limit is the
// interesting part. The server's label is a SQL string assembled from tokens
// and bound at query time; proving the two produce identical text would mean
// executing it against Postgres, and this package has no database — the
// integration lane does. So this gate holds the two DECISIONS that can be read
// off the source, and they are the two that carry the whole rule:
//
//  1. January is spelled as a bare calendar year on both sides, and every other
//     month is spelled as a two-year span. That branch is the entire product
//     decision, and getting it wrong is the failure that reaches a reader.
//  2. The span names the starting year in full and the ending year in two
//     digits, in that order, separated by "/", behind an "FY" prefix.
//
// What it cannot see: an off-by-one in the SQL's month shift, or a to_char
// pattern that renders the right shape from the wrong instant. Those are
// arithmetic rather than shape, and TestQuarterBounds plus the report
// integration tests are what hold them.

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

const (
	frontendFiscalYear = "../frontend/src/format/fiscalyear.ts"
	backendFiscalYear  = "internal/compose/reportperiod.go"
)

func readFiscalSource(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return string(body)
}

// TestFiscalYearLabelIsSpelledTheSameOnBothSidesOfTheWire holds the mirror.
func TestFiscalYearLabelIsSpelledTheSameOnBothSidesOfTheWire(t *testing.T) {
	frontend := readFiscalSource(t, frontendFiscalYear)
	backend := readFiscalSource(t, backendFiscalYear)

	t.Run("both branch on January", func(t *testing.T) {
		// The server compares the bound month against 1; the browser compares
		// its argument against 1. Either side losing this branch is the
		// regression that matters most, because it changes what every EXISTING
		// installation sees — all of them run on the January default.
		if !strings.Contains(backend, `reportFiscalStartMonthToken + " = 1"`) {
			t.Error("the SQL no longer branches on a January start; every calendar-year installation would be relabelled")
		}
		if !regexp.MustCompile(`startMonth === 1`).MatchString(frontend) {
			t.Error("the TypeScript no longer branches on a January start")
		}
	})

	t.Run("both spell the span FY<full>/<two>", func(t *testing.T) {
		// The server concatenates 'FY', a YYYY, '/', and a YY.
		wantBackend := []string{`"'FY' || to_char("`, `" || '/' || to_char("`, `'YY'`}
		for _, fragment := range wantBackend {
			if !strings.Contains(backend, fragment) {
				t.Errorf("the SQL no longer builds the FY<full>/<two> span: missing %s", fragment)
			}
		}
		// The browser builds the same shape as a template literal.
		if !strings.Contains(frontend, "`FY${startYear}/${ends}`") {
			t.Error("the TypeScript no longer builds the FY<full>/<two> span")
		}
		// And derives the second half by ADDING a year rather than slicing the
		// first, which is what makes 2099 roll to 00 rather than to 100.
		if !strings.Contains(frontend, "(startYear + 1) % 100") {
			t.Error("the TypeScript no longer derives the ending year by addition; the century would not roll over")
		}
		if !strings.Contains(backend, `+ interval '1 year'`) {
			t.Error("the SQL no longer derives the ending year by addition")
		}
	})

	t.Run("the quarter hangs off the year label", func(t *testing.T) {
		// A quarter is the year's own label plus "-Qn", so the two can never
		// disagree about the year part.
		if !strings.Contains(backend, `" || '-Q' || to_char("`) {
			t.Error("the SQL quarter no longer extends the fiscal year label")
		}
	})
}
