// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contactbrief

// Every language the product ships writes the whole floor, with the same slots.
//
// The floor is what a deployment serves when no model answers, so a language
// missing from this table is an installation whose cards silently change
// language the moment its lane fails. A missing FIELD is worse: Go's zero value
// for a string is "", so the sentence would not be wrong, it would be absent.

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
	english, ok := briefCopy[textlang.English]
	if !ok {
		t.Fatal("English is absent from the floor's copy table, so there is nothing to compare against")
	}
	shape := reflect.TypeOf(english)
	if shape.NumField() == 0 {
		t.Fatal("the phrase struct has no fields; this census would certify nothing")
	}

	for _, lang := range textlang.Shipped {
		phrases, ok := briefCopy[lang]
		if !ok {
			t.Errorf("%s ships in this product but writes no deterministic contact floor, so a "+
				"card in that installation changes language whenever the model lane fails", lang)
			continue
		}
		got, want := reflect.ValueOf(phrases), reflect.ValueOf(english)
		for i := range shape.NumField() {
			name := shape.Field(i).Name
			text := got.Field(i).String()
			if strings.TrimSpace(text) == "" {
				t.Errorf("%s leaves %s empty, which renders as a missing sentence rather than a "+
					"wrong one", lang, name)
				continue
			}
			gotVerbs := verbs.FindAllString(text, -1)
			wantVerbs := verbs.FindAllString(want.Field(i).String(), -1)
			if !reflect.DeepEqual(gotVerbs, wantVerbs) {
				t.Errorf("%s writes %s with placeholders %v, but the sentence is given %v.\n"+
					"  %s\nA dropped placeholder renders as %%!s(MISSING) in a card; an extra one "+
					"reads an argument nobody passed.", lang, name, gotVerbs, wantVerbs, text)
			}
		}
	}
}

// An unknown code answers in English rather than in empty strings. It reaches
// here from a stored setting this build no longer ships, which is the same case
// BaseLanguageForPrompt itself falls back to English for.
func TestAnUnshippedLanguageFallsBackRatherThanBlank(t *testing.T) {
	if got := phrasesFor("kl"); got.IdentityBare != briefCopy[textlang.English].IdentityBare {
		t.Fatalf("an unshipped language answered %q, want the English floor", got.IdentityBare)
	}
}

// The floor actually writes in the language it is handed — the whole point, and
// the thing a table alone does not prove.
func TestTheFloorWritesInTheLanguageItIsGiven(t *testing.T) {
	in := inputFixture()
	for _, lang := range textlang.Shipped {
		prose := Prose(Deterministic(briefContactID, in, string(lang)))
		want := fmt.Sprintf(briefCopy[lang].IdentityTitleEmployer, in.Name, in.Title, in.Employer)
		if !strings.Contains(prose, want) {
			t.Errorf("the %s floor does not open with its own identity sentence.\n got: %s\nwant: %s",
				lang, prose, want)
		}
	}
}
