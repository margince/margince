// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftfloor_test

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// The German weeks band opens under the greeting in lower case. The sentence of
// substance after it is a new sentence, so it opens on a capital.
func TestOnlyTheGermanParagraphUnderTheGreetingOpensInLowerCase(t *testing.T) {
	phrases := draftfloor.For(textlang.German, convstate.BandWeeks)
	thread := draftfloor.Fill(draftfloor.SubstanceFor(textlang.German).Thread, "Nachfrage Verlängerung")

	got := draftfloor.Body(textlang.German, "Hallo Jonas,", phrases.Opener, thread, phrases.Ask)

	want := "Hallo Jonas,\n\n" +
		"seit unserem letzten Austausch sind einige Wochen vergangen, deshalb hier der aktuelle Stand.\n\n" +
		"Ich wollte zu Nachfrage Verlängerung nachfassen.\n\n" +
		"Wäre ein kurzes Gespräch in den nächsten zwei Wochen sinnvoll?"
	if got != want {
		t.Fatalf("body:\n%s\nwant:\n%s", got, want)
	}
}

func TestAGermanSubstanceDirectlyUnderTheGreetingStaysLowerCase(t *testing.T) {
	thread := draftfloor.Fill(draftfloor.SubstanceFor(textlang.German).Thread, "Angebot")

	got := draftfloor.Body(textlang.German, "Hallo Jonas,", "", thread, "Passt Ihnen ein kurzes Gespräch?")

	want := "Hallo Jonas,\n\nich wollte zu Angebot nachfassen.\n\nPasst Ihnen ein kurzes Gespräch?"
	if got != want {
		t.Fatalf("body:\n%s\nwant:\n%s", got, want)
	}
}

// Vietnamese opens a new line after the greeting on a capital, as the table's
// own openers do.
func TestAVietnameseParagraphAlwaysOpensOnACapital(t *testing.T) {
	thread := draftfloor.Fill(draftfloor.SubstanceFor(textlang.Vietnamese).Thread, "hợp đồng")

	got := draftfloor.Body(textlang.Vietnamese, "Chào Lan,", thread)

	if want := "Chào Lan,\n\nTôi xin phép trao đổi tiếp về hợp đồng."; got != want {
		t.Fatalf("body:\n%s\nwant:\n%s", got, want)
	}
}
