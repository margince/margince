// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// Every shipped language reads a technical change in ITS OWN words.
//
// The bug this censuses against was real: the summaries were written in one
// language and stored, so every other installation read them in a language it
// never chose. An entry alone is not enough — a set copied from English and
// never translated is the same reader in the same wrong language — so the
// sentence frames are asserted different, not just present. The service names
// are not: a name can legitimately coincide across languages ("Webshop" is
// both the German and the English word), so for names the census asks for
// coverage of the owner's key set instead.

import (
	"strings"
	"testing"

	"github.com/margince/margince/backend/internal/platform/techprofile"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// frames flattens one language's sentence frames beside how many value holes
// each must keep: a %s dropped in translation prints a sentence without the
// thing the sentence is about.
func frames(said technicalSummaryCopy) []struct {
	name, frame string
	holes       int
} {
	return []struct {
		name, frame string
		holes       int
	}{
		{"mailMoved", said.mailMoved, 2},
		{"mailSet", said.mailSet, 1},
		{"serviceGone", said.serviceGone, 1},
		{"serviceNew", said.serviceNew, 1},
		{"hostingMoved", said.hostingMoved, 2},
		{"hostingSet", said.hostingSet, 1},
		{"technologyGone", said.technologyGone, 1},
		{"technologyNew", said.technologyNew, 1},
	}
}

func TestEveryShippedLanguageSummarisesTechnicalChanges(t *testing.T) {
	english, ok := technicalSummaryByLang[textlang.English]
	if !ok {
		t.Fatal("English has no technical summary set, and it is the fallback for everything else")
	}
	for _, lang := range textlang.Shipped {
		t.Run(string(lang), func(t *testing.T) {
			said, ok := technicalSummaryByLang[lang]
			if !ok {
				t.Fatalf("the product ships %s and this table has not learned it", lang)
			}
			for _, field := range frames(said) {
				if field.frame == "" {
					t.Errorf("%s is empty — a blank frame reaches the reader as nothing at all", field.name)
					continue
				}
				if got := strings.Count(field.frame, "%s"); got != field.holes {
					t.Errorf("%s carries %d %%s holes, want %d: %q", field.name, got, field.holes, field.frame)
				}
				if lang != textlang.English && field.frame == englishFrame(english, field.name) {
					t.Errorf("%s is the English frame verbatim; the entry exists and the reader still gets English", field.name)
				}
			}
			for _, key := range techprofile.ServiceKeys() {
				if said.serviceNames[key] == "" {
					t.Errorf("service %q has no %s name — its sentence would carry the record's label in whatever language the record stored", key, lang)
				}
			}
			for _, key := range []string{techprofile.MailSelfHosted, techprofile.MailOther} {
				if said.mailNames[key] == "" {
					t.Errorf("mail fallback %q has no %s name — the one mail label that is prose rather than a vendor name", key, lang)
				}
			}
		})
	}
}

func englishFrame(english technicalSummaryCopy, name string) string {
	for _, field := range frames(english) {
		if field.name == name {
			return field.frame
		}
	}
	return ""
}

// A vendor name must survive untouched: the mail map holds only the two prose
// fallbacks, so a real provider's label passes through as recorded.
func TestVendorNamesAreNeverTranslated(t *testing.T) {
	for _, lang := range textlang.Shipped {
		said := technicalSummaryByLang[lang]
		if got := nameOr(said.mailNames, "microsoft365", "Microsoft 365"); got != "Microsoft 365" {
			t.Errorf("%s renames Microsoft 365 to %q", lang, got)
		}
	}
}
