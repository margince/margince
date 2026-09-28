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
	"reflect"
	"regexp"
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// verbs counts the format placeholders in a template. A translation that drops
// one does not fail to compile — it renders "%!s(MISSING)" into a card — and a
// translation that adds one consumes an argument nobody passed.
var verbs = regexp.MustCompile(`%[a-zA-Z]|%%`)

func TestEveryShippedLanguageWritesTheContactFloor(t *testing.T) {
	shape := reflect.TypeOf(floor)
	if shape.NumField() == 0 {
		t.Fatal("the phrase struct has no fields; this census would certify nothing")
	}
	value := reflect.ValueOf(floor)

	for i := range shape.NumField() {
		name := shape.Field(i).Name
		p := value.Field(i).Interface().(phrase)
		english := p.in(textlang.English)
		if strings.TrimSpace(english) == "" {
			t.Errorf("%s has no English sentence, so there is nothing to translate against", name)
			continue
		}
		for _, lang := range textlang.Shipped {
			text := p.in(lang)
			if strings.TrimSpace(text) == "" {
				t.Errorf("%s leaves %s unwritten, which renders as a missing sentence rather than "+
					"a wrong one", lang, name)
				continue
			}
			if got, want := verbs.FindAllString(text, -1), verbs.FindAllString(english, -1); !reflect.DeepEqual(got, want) {
				t.Errorf("%s writes %s with placeholders %v, but the sentence is given %v.\n"+
					"  %s\nA dropped placeholder renders as %%!s(MISSING) in a card; an extra one "+
					"reads an argument nobody passed.", lang, name, got, want, text)
			}
		}
	}
}

// A count of one gets its own sentence. "1 days" and "1 Tagen" both read as a
// machine talking, and the second is not German at all.
func TestASingleDayIsNotWrittenAsAPlural(t *testing.T) {
	for _, lang := range textlang.Shipped {
		for name, singular := range map[string]phrase{
			"AnsweredAfterADay": floor.AnsweredAfterADay,
			"QuietForADay":      floor.QuietForADay,
		} {
			if got := verbs.FindAllString(singular.in(lang), -1); len(got) != 0 {
				t.Errorf("%s writes %s with %v — a sentence about exactly one day takes no count",
					lang, name, got)
			}
		}
	}
}

// An unknown code answers in English rather than in empty strings. It reaches
// here from a stored setting this build no longer ships, which is the same case
// BaseLanguageForPrompt itself falls back to English for.
func TestAnUnshippedLanguageFallsBackRatherThanBlank(t *testing.T) {
	if got := phrasesFor("kl").say(floor.IdentityBare); got != floor.IdentityBare.in(textlang.English) {
		t.Fatalf("an unshipped language answered %q, want the English floor", got)
	}
}

// The floor actually writes in the language it is handed — the whole point, and
// the thing a table alone does not prove.
func TestTheFloorWritesInTheLanguageItIsGiven(t *testing.T) {
	in := inputFixture()
	for _, lang := range textlang.Shipped {
		prose := Prose(Deterministic(briefContactID, in, string(lang)))
		want := fmt.Sprintf(floor.IdentityTitleEmployer.in(lang), in.Name, in.Title, in.Employer)
		if !strings.Contains(prose, want) {
			t.Errorf("the %s floor does not open with its own identity sentence.\n got: %s\nwant: %s",
				lang, prose, want)
		}
	}
}
