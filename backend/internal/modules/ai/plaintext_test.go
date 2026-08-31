// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package ai

import (
	"testing"
)

func TestABreakTagBecomesTheLineBreakItMeant(t *testing.T) {
	// The observed shape: a model asked for a German reply answered with
	// `<br><br>` between paragraphs, and every one of those eight characters
	// reached the composer's textarea.
	got := PlainText("Guten Tag Dietmar,<br><br>anbei die Aufstellung.<br><br>Viele Grüße")
	want := "Guten Tag Dietmar,\n\nanbei die Aufstellung.\n\nViele Grüße"
	if got != want {
		t.Errorf("PlainText = %q, want %q", got, want)
	}
}

func TestTheBreakIsHonouredRatherThanDeleted(t *testing.T) {
	// Dropping the tag would run two paragraphs into one sentence, which is
	// the same defect with tidier bytes. The paragraph the model wrote has to
	// survive as a paragraph.
	got := PlainText("Erste Zeile.<br>Zweite Zeile.")
	if got != "Erste Zeile.\nZweite Zeile." {
		t.Errorf("PlainText = %q, want the break kept as a newline", got)
	}
}

func TestEverySpellingOfABreakTagIsRecognised(t *testing.T) {
	// A model writes the tag however its training data did. Recognising only
	// one spelling fixes the drafts that happen to use it and leaves the rest
	// broken in a way nobody can tell apart from the fix not shipping.
	for _, tag := range []string{"<br>", "<BR>", "<br/>", "<br />", "<br  />", "<Br>"} {
		if got := PlainText("eins" + tag + "zwei"); got != "eins\nzwei" {
			t.Errorf("PlainText with %q = %q, want eins\\nzwei", tag, got)
		}
	}
}

func TestParagraphMarkupBecomesParagraphs(t *testing.T) {
	got := PlainText("<p>Erster Absatz.</p><p>Zweiter Absatz.</p>")
	if got != "Erster Absatz.\n\nZweiter Absatz." {
		t.Errorf("PlainText = %q, want two paragraphs", got)
	}
}

func TestTextTheReaderWroteIsNotEatenByTheConversion(t *testing.T) {
	// The reason this translates break tags rather than stripping markup: a
	// body legitimately carries "<". A general tag stripper would silently
	// delete the customer's own words to fix our formatting.
	for _, body := range []string{
		"Der Betrag ist < 500 EUR.",
		"Bitte an dietmar <dietmar@example.com> weiterleiten.",
		"Er schrieb: <Das ist nicht vereinbart>.",
		"a < b und b > c",
	} {
		if got := PlainText(body); got != body {
			t.Errorf("PlainText(%q) = %q, want it unchanged", body, got)
		}
	}
}

func TestPlainTextLeavesPlainTextAlone(t *testing.T) {
	body := "Guten Tag,\n\nanbei die Aufstellung.\n\nViele Grüße"
	if got := PlainText(body); got != body {
		t.Errorf("PlainText = %q, want the body unchanged", got)
	}
}

func TestRunsOfBreaksCollapseToOneParagraph(t *testing.T) {
	// A model that emits four breaks means one paragraph gap, not three blank
	// lines the reader has to delete before sending.
	got := PlainText("eins<br><br><br><br>zwei")
	if got != "eins\n\nzwei" {
		t.Errorf("PlainText = %q, want a single paragraph break", got)
	}
}
