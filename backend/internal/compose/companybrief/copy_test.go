// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companybrief

// Every language the product ships writes the whole company floor.

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
	shape := reflect.TypeOf(floor)
	value := reflect.ValueOf(floor)
	for i := range shape.NumField() {
		name := shape.Field(i).Name
		if table, ok := value.Field(i).Interface().(map[string]phrase); ok {
			for key, p := range table {
				checkPhrase(t, name+"["+key+"]", p)
			}
			continue
		}
		checkPhrase(t, name, value.Field(i).Interface().(phrase))
	}
}

func checkPhrase(t *testing.T, name string, p phrase) {
	t.Helper()
	english := p.in(textlang.English)
	if strings.TrimSpace(english) == "" {
		t.Errorf("%s has no English sentence, so there is nothing to translate against", name)
		return
	}
	for _, lang := range textlang.Shipped {
		text := p.in(lang)
		if strings.TrimSpace(text) == "" {
			t.Errorf("%s leaves %s unwritten, which renders as a missing sentence", lang, name)
			continue
		}
		if name == "DateLayout" {
			continue // a Go reference layout, not a format string
		}
		if got, want := companyVerbs.FindAllString(text, -1), companyVerbs.FindAllString(english, -1); !reflect.DeepEqual(got, want) {
			t.Errorf("%s writes %s with placeholders %v, but the sentence is given %v.\n  %s",
				lang, name, got, want, text)
		}
	}
}

// englishMonths are what time.Format writes for the `Jan` token — in English,
// whatever language the rest of the sentence is in.
var englishMonths = []string{
	"Jan", "Feb", "Mar", "Apr", "May", "Jun",
	"Jul", "Aug", "Sep", "Oct", "Nov", "Dec",
}

// A date layout may only name a month in words in English.
//
// Go's time package has no locale: the `Jan` token always renders an English
// abbreviation, so a German layout that spells the month writes "2. May. 2026"
// — an English word in a German sentence. The other languages are numeric for
// that reason.
//
// Every month, not one: an earlier version of this test formatted a January
// date, and "Jan" is spelled the same in English and German, so it passed while
// eleven other months were wrong.
func TestNoLanguageWritesAnEnglishMonthName(t *testing.T) {
	for _, lang := range textlang.Shipped {
		if lang == textlang.English {
			continue
		}
		layout := floor.DateLayout.in(lang)
		for month := time.January; month <= time.December; month++ {
			rendered := time.Date(2026, month, 2, 0, 0, 0, 0, time.UTC).Format(layout)
			for _, name := range englishMonths {
				if strings.Contains(rendered, name) {
					t.Errorf("the %s date layout renders %s as %q, which carries the English "+
						"month %q — time.Format has no locale, so a layout naming the month in "+
						"words writes English into every other language",
						lang, month, rendered, name)
				}
			}
		}
	}
}

// A date layout still has to render a date, or the floor prints the layout back
// at the reader.
func TestEveryLanguageDateLayoutRendersADate(t *testing.T) {
	when := time.Date(2026, time.November, 2, 15, 4, 5, 0, time.UTC)
	for _, lang := range textlang.Shipped {
		layout := floor.DateLayout.in(lang)
		got := when.Format(layout)
		if !strings.Contains(got, "2026") || got == layout {
			t.Errorf("%s renders the reference instant as %q, which is not a date", lang, got)
		}
	}
}

// A count of one takes its own sentence: "über 1 bekannte Kontakte" is not
// German, and the same shape is wrong in every language that inflects.
func TestASingleKnownContactIsNotWrittenAsAPlural(t *testing.T) {
	for _, lang := range textlang.Shipped {
		if got := companyVerbs.FindAllString(floor.StrengthOverOne.in(lang), -1); len(got) != 1 {
			t.Errorf("%s writes StrengthOverOne with %v — it takes the strength and nothing else, "+
				"because the count it would print is always one", lang, got)
		}
	}
}

func TestAnUnshippedLanguageFallsBackToTheEnglishCompanyFloor(t *testing.T) {
	if got := companyPhrasesFor("kl").say(floor.OpenDealOne); got != floor.OpenDealOne.in(textlang.English) {
		t.Fatalf("an unshipped language answered %q, want the English floor", got)
	}
}
