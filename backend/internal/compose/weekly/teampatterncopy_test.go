// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package weekly

import (
	"slices"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestWeeklyPatternCopyCensus(t *testing.T) {
	langcopytest.Census(t, patternWords)
	langcopytest.NoCount(t, map[string]langcopy.Phrase{
		"help": patternWords.HelpOne, "lead": patternWords.BreachOne, "won": patternWords.WonOne,
		"meeting": patternWords.MeetingOne, "forecast": patternWords.ForecastOne, "regression": patternWords.RegressedOne,
	})
}

func TestWeeklyPatternLanguageKeepsRulesAndPriority(t *testing.T) {
	counts := Counts{LeadsBreached: 1, CommitmentsDue: 2, CommitmentsKept: 1, DealsWon: 1, MeetingsHeld: 2, MeetingsWithNextStep: 1}
	deals := &DealBlock{ForecastDown: 1, Regressions: 1, Open: 4, WithNextStep: 1, CloseDateProvisional: new(2)}
	english := teamCopyFor("en").patternsFor(counts, 1, deals)
	for _, language := range textlang.Shipped {
		got := teamCopyFor(string(language)).patternsFor(counts, 1, deals)
		if !slices.Equal(kinds(got), kinds(english)) {
			t.Fatalf("%s changed priority", language)
		}
		seen := map[string]bool{FocusQuietWeek: true}
		for i, pattern := range got {
			seen[pattern.Kind] = true
			if strings.Contains(pattern.Label, "%!") || (language != textlang.English && pattern.Label == english[i].Label) {
				t.Fatalf("%s: %s", language, pattern.Label)
			}
		}
		for _, kind := range focusPriority {
			if kind != FocusDealsAtRisk && !seen[kind] {
				t.Errorf("%s did not exercise %s", language, kind)
			}
		}
		quiet := teamCopyFor(string(language)).patternsFor(Counts{}, 0, nil)
		if len(quiet) != 1 || quiet[0].Kind != FocusQuietWeek || quiet[0].Label == "" {
			t.Fatalf("quiet %s: %+v", language, quiet)
		}
	}
}
