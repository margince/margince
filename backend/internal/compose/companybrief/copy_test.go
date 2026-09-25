// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companybrief

// Every language the product ships writes the whole company floor.
//
// A language missing from the table is an installation whose card changes
// language the moment its model lane fails. A missing FIELD is worse: Go's zero
// value for a string is "", so the sentence is absent rather than wrong.

import (
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

var companyVerbs = regexp.MustCompile(`%[a-zA-Z]|%%`)

func TestEveryShippedLanguageWritesTheCompanyFloor(t *testing.T) {
	english, ok := companyCopy[textlang.English]
	if !ok {
		t.Fatal("English is absent from the company floor's copy table")
	}
	shape := reflect.TypeOf(english)
	for _, lang := range textlang.Shipped {
		phrases, ok := companyCopy[lang]
		if !ok {
			t.Errorf("%s ships in this product but writes no deterministic company floor", lang)
			continue
		}
		got, want := reflect.ValueOf(phrases), reflect.ValueOf(english)
		for i := range shape.NumField() {
			name := shape.Field(i).Name
			if shape.Field(i).Type.Kind() == reflect.Map {
				if got.Field(i).Len() != want.Field(i).Len() {
					t.Errorf("%s names %d entries in %s, English names %d — a key missing here "+
						"drops the sentence rather than translating it",
						lang, got.Field(i).Len(), name, want.Field(i).Len())
				}
				continue
			}
			text := got.Field(i).String()
			if strings.TrimSpace(text) == "" {
				t.Errorf("%s leaves %s empty, which renders as a missing sentence", lang, name)
				continue
			}
			if name == "DateLayout" {
				continue // a Go reference layout, not a format string
			}
			gotVerbs := companyVerbs.FindAllString(text, -1)
			wantVerbs := companyVerbs.FindAllString(want.Field(i).String(), -1)
			if !reflect.DeepEqual(gotVerbs, wantVerbs) {
				t.Errorf("%s writes %s with placeholders %v, but the sentence is given %v.\n  %s",
					lang, name, gotVerbs, wantVerbs, text)
			}
		}
	}
}

// A date layout has to render the reference instant as a real date, or the
// floor prints the layout back at the reader.
func TestEveryLanguageDateLayoutRendersADate(t *testing.T) {
	when := time.Date(2026, time.January, 2, 15, 4, 5, 0, time.UTC)
	for _, lang := range textlang.Shipped {
		got := when.Format(companyCopy[lang].DateLayout)
		if !strings.Contains(got, "2026") || got == companyCopy[lang].DateLayout {
			t.Errorf("%s renders the reference instant as %q, which is not a date", lang, got)
		}
	}
}

func TestAnUnshippedLanguageFallsBackToTheEnglishCompanyFloor(t *testing.T) {
	if got := companyPhrasesFor("kl"); got.OpenDealOne != companyCopy[textlang.English].OpenDealOne {
		t.Fatalf("an unshipped language answered %q, want the English floor", got.OpenDealOne)
	}
}
