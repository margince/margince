// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package langcopy_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// A sentence is answered in the language it is asked for.
func TestAPhraseAnswersInTheLanguageAsked(t *testing.T) {
	t.Parallel()
	p := langcopy.Phrase{En: "Nothing needs you today", De: "Heute braucht dich nichts", Vi: "Hôm nay không có việc gì cần bạn"}
	for lang, want := range map[textlang.Lang]string{
		textlang.English:    "Nothing needs you today",
		textlang.German:     "Heute braucht dich nichts",
		textlang.Vietnamese: "Hôm nay không có việc gì cần bạn",
	} {
		if got := p.In(lang); got != want {
			t.Errorf("%s answered %q, want %q", lang, got, want)
		}
	}
}

// An unshipped code answers in English rather than in an empty string. It
// reaches here from a stored setting this build no longer ships, which is the
// case BaseLanguageForPrompt itself falls back to English for.
func TestAnUnshippedCodeFallsBackToEnglish(t *testing.T) {
	t.Parallel()
	p := langcopy.Phrase{En: "Due today.", De: "Heute fällig.", Vi: "Đến hạn hôm nay."}
	if got := langcopy.For("kl").Say(p); got != p.En {
		t.Fatalf("an unshipped code answered %q, want the English sentence", got)
	}
	if got := langcopy.For("").Say(p); got != p.En {
		t.Fatalf("an empty code answered %q, want the English sentence", got)
	}
}

// The resolved language is readable, because a caller that formats a date needs
// to know which month table to reach for.
func TestASpokenNamesItsLanguage(t *testing.T) {
	t.Parallel()
	if got := langcopy.For(string(textlang.German)).Lang(); got != textlang.German {
		t.Fatalf("a German Spoken names itself %q", got)
	}
}
