// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactbrief

// Every language the product ships writes the whole floor, with the same slots.
//
// The floor is what a deployment serves when no model answers, so a sentence
// left unwritten in one language is an installation whose card silently changes
// language. Go fills an omitted field of a keyed literal with "", so the
// sentence goes MISSING rather than arriving wrong — which is why this counts
// fields rather than trusting the compiler to have asked for them.

import (
	"fmt"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy/langcopytest"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

func TestEveryShippedLanguageWritesTheContactFloor(t *testing.T) {
	t.Parallel()
	langcopytest.Census(t, floor)
}

// A count of one gets its own sentence. "1 days" and "1 Tagen" both read as a
// machine talking, and the second is not German at all.
func TestASingleDayIsNotWrittenAsAPlural(t *testing.T) {
	t.Parallel()
	langcopytest.NoCount(t, map[string]phrase{
		"AnsweredAfterADay": floor.AnsweredAfterADay,
		"QuietForADay":      floor.QuietForADay,
	})
}

// An unknown code answers in English rather than in empty strings. It reaches
// here from a stored setting this build no longer ships, which is the same case
// BaseLanguageForPrompt itself falls back to English for.
func TestAnUnshippedLanguageFallsBackRatherThanBlank(t *testing.T) {
	if got := phrasesFor("kl").Say(floor.IdentityBare); got != floor.IdentityBare.In(textlang.English) {
		t.Fatalf("an unshipped language answered %q, want the English floor", got)
	}
}

// The floor actually writes in the language it is handed — the whole point, and
// the thing a table alone does not prove.
func TestTheFloorWritesInTheLanguageItIsGiven(t *testing.T) {
	in := inputFixture()
	for _, lang := range textlang.Shipped {
		prose := Prose(Deterministic(briefContactID, in, string(lang)))
		want := fmt.Sprintf(floor.IdentityTitleEmployer.In(lang), in.Name, in.Title, in.Employer)
		if !strings.Contains(prose, want) {
			t.Errorf("the %s floor does not open with its own identity sentence.\n got: %s\nwant: %s",
				lang, prose, want)
		}
	}
}
