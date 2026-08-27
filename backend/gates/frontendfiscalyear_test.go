// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H1

package gates

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
// arithmetic rather than shape, and they are held by TestQuarterBounds and by
// TestWinLossPeriodBucketsFollowTheInstallationFiscalYear /
// TestTheFiscalMonthShiftIsExactAtTheYearsFirstDay in
// internal/compose/report_winloss_integration_test.go, which execute the real
// statement against Postgres. The second of those exists BECAUSE this gate
// could not see the shift: dropping the `- 1` left every assertion here green
// while an April-start installation labelled its year one year early.
//
// It also matches SOURCE FRAGMENTS, which is a real limitation in the other
// direction: an innocuous refactor that extracts `'FY'` to a constant or
// reorders the concatenation fails it, even though the behaviour is unchanged.
// That is the trade a shape gate makes, and the failure is loud and obvious
// rather than silent — fix the fragments here, and let the integration tests
// above say whether the behaviour actually moved.

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

// readFiscalSource reads a file with its COMMENTS REMOVED.
//
// Load-bearing, not tidiness. Every fragment this gate matches is prose-shaped
// enough to survive being quoted in a comment, so a plain read would let the
// gate be satisfied by its own subject's obituary: rewrite the January branch
// to `= 99` and leave `// was: ... " = 1"` above it, and the gate stays green
// over code that no longer does what it checks. Verified by doing exactly that
// before this existed.
//
// Blanked rather than deleted, keeping the newlines, so a finding's line number
// still points at the code rather than at whatever the collapse shifted into
// its place.
func readFiscalSource(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("reading %s: %v", path, err)
	}
	return blankComments(string(body))
}

// blankComments replaces every // and /* */ comment with spaces, preserving
// newlines and the length of the file.
//
// A scanner rather than a regexp, because a comment opener is not something a
// pattern can settle: `//` sits inside every URL, and both openers sit inside
// ordinary string literals — this very file contains `"'FY' || to_char("`. A
// pattern that paired an opener with the next terminator would blank real code
// and report success over lines it never read, which is worse than a false
// finding: a finding gets looked at, a blind spot reports PASS. The frontend's
// zone-by-purpose gate reaches for the TypeScript parser for the same reason.
func blankComments(source string) string {
	out := []rune(source)
	const (
		code = iota
		lineComment
		blockComment
		stringLit
		rawStringLit
		runeLit
	)
	state := code
	blank := func(i int) {
		if out[i] != '\n' {
			out[i] = ' '
		}
	}
	for i := 0; i < len(out); i++ {
		c := out[i]
		next := rune(0)
		if i+1 < len(out) {
			next = out[i+1]
		}
		switch state {
		case code:
			switch {
			case c == '/' && next == '/':
				state = lineComment
				blank(i)
			case c == '/' && next == '*':
				state = blockComment
				blank(i)
			case c == '"':
				state = stringLit
			case c == '`':
				state = rawStringLit
			case c == '\'':
				state = runeLit
			}
		case lineComment:
			if c == '\n' {
				state = code
			} else {
				blank(i)
			}
		case blockComment:
			blank(i)
			if c == '*' && next == '/' {
				blank(i + 1)
				i++
				state = code
			}
		case stringLit:
			switch c {
			// An escaped quote does not close the literal.
			case '\\':
				i++
			case '"':
				state = code
			}
		case rawStringLit:
			if c == '`' {
				state = code
			}
		case runeLit:
			switch c {
			case '\\':
				i++
			case '\'':
				state = code
			}
		}
	}
	return string(out)
}

// TestFiscalYearLabelIsSpelledTheSameOnBothSidesOfTheWire holds the mirror.
func TestFiscalYearLabelIsSpelledTheSameOnBothSidesOfTheWire(t *testing.T) {
	t.Parallel()
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

// TestTheGateReadsCodeRatherThanComments is the gate's own acceptance test.
//
// blankComments is what stops a fragment migrating into a comment from
// satisfying an assertion about code, and a stripper that quietly stopped
// stripping would take the gate with it — silently, since every assertion
// would keep passing. So the two directions are both planted here: a fragment
// in a comment must NOT be seen, and the same fragment in code must be.
func TestTheGateReadsCodeRatherThanComments(t *testing.T) {
	t.Parallel()
	source := "package p\n" +
		"// was: reportFiscalStartMonthToken + \" = 1\"\n" +
		"/* also: reportFiscalStartMonthToken + \" = 1\" */\n" +
		"const url = \"https://example.test/a//b\"\n" +
		"const live = reportFiscalStartMonthToken + \" = 99\"\n"

	stripped := blankComments(source)

	if strings.Contains(stripped, `" = 1"`) {
		t.Error("a fragment inside a comment survived the strip, so the gate can be satisfied by prose")
	}
	if !strings.Contains(stripped, `" = 99"`) {
		t.Error("the strip ate real code")
	}
	// The URL's `//` is inside a string literal and opens no comment; a
	// pattern-based stripper blanks the rest of that line and every line under
	// it, which is how a gate goes blind while reporting PASS.
	if !strings.Contains(stripped, "https://example.test/a//b") {
		t.Error("a URL inside a string literal was treated as a comment opener")
	}
	// Line numbers must survive, or a finding points at innocent code.
	if strings.Count(stripped, "\n") != strings.Count(source, "\n") {
		t.Error("the strip changed the line count")
	}
}
