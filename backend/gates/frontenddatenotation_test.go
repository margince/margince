// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

//gate:kind parity H3

package gates

// A server-written brief and the app around it must write one day the same
// way, or the reader sees 16/11/2026 beside 2026-11-16 on one screen.
//
// The browser owns the patterns: preferences.ts names the fixed ones, and its
// own tests pin what each language's "locale" pattern writes. datenotation is
// the declared mirror. This compares both directions, so a pattern added,
// dropped or changed on either side fails.

import (
	"regexp"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/gatekit"
	"github.com/margince/margince/backend/internal/shared/kernel/datenotation"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

const (
	frontendDatePreferences = "../frontend/src/format/preferences.ts"
	frontendDatePatternTest = "../frontend/src/format/datepattern.test.ts"
)

// tsFixedPattern reads one `dmy: { order: ["day", "month", "year"], separator: "." }` entry.
var tsFixedPattern = regexp.MustCompile(`(\w+):\s*\{\s*order:\s*\[\s*"(\w+)",\s*"(\w+)",\s*"(\w+)"\s*\],\s*separator:\s*"([^"]*)"\s*\}`)

// tsLocaleCase reads one `["en", "locale", "06/05/2026"]` row of the pattern test.
var tsLocaleCase = regexp.MustCompile(`\[\s*"(\w+)",\s*"locale",\s*"([^"]+)"\s*\]`)

func TestTheFrontendDatePatternsMatchTheGoOnes(t *testing.T) {
	t.Parallel()
	source, err := gatekit.ReadFile(frontendDatePreferences)
	if err != nil {
		t.Fatalf("reading the frontend date patterns: %v", err)
	}
	inTS := map[string]datenotation.Pattern{}
	for _, m := range tsFixedPattern.FindAllStringSubmatch(tsComment.ReplaceAllString(string(source), " "), -1) {
		inTS[m[1]] = datenotation.Pattern{Order: [3]string{m[2], m[3], m[4]}, Separator: m[5]}
	}
	inGo := datenotation.FixedPatterns()
	if len(inTS) == 0 {
		t.Fatal("no fixed pattern parsed out of the frontend: a gate that reads nothing agrees with everything")
	}
	for name, pattern := range inGo {
		if inTS[name] != pattern {
			t.Errorf("%s: Go writes %+v, the frontend %+v", name, pattern, inTS[name])
		}
	}
	for name := range inTS {
		if _, ok := inGo[name]; !ok {
			t.Errorf("%s: the frontend names a pattern datenotation does not have", name)
		}
	}
}

func TestTheFrontendLocaleDatesMatchTheGoOnes(t *testing.T) {
	t.Parallel()
	source, err := gatekit.ReadFile(frontendDatePatternTest)
	if err != nil {
		t.Fatalf("reading the frontend date pattern test: %v", err)
	}
	pinned := map[textlang.Lang]string{}
	for _, m := range tsLocaleCase.FindAllStringSubmatch(string(source), -1) {
		pinned[textlang.Lang(m[1])] = m[2]
	}
	inGo := datenotation.LocalePatterns()
	if len(pinned) != len(inGo) {
		t.Fatalf("the frontend pins %d locale dates and Go has %d locale patterns; each language needs both", len(pinned), len(inGo))
	}
	for lang, want := range pinned {
		day := datenotation.Notation{Lang: lang, Date: "locale"}.CalendarDay(mayTheSixth)
		if day != want {
			t.Errorf("%s: Go writes 6 May 2026 as %q, the frontend as %q", lang, day, want)
		}
	}
}

var mayTheSixth = time.Date(2026, time.May, 6, 12, 0, 0, 0, time.UTC)
