// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftfloor_test

// Putting the opening greeting on its own line.

import (
	"testing"

	"github.com/margince/margince/backend/internal/shared/kernel/draftfloor"
)

// The defect this exists for: greeting and message on one line.
func TestAGreetingRunningIntoTheMessageIsSplit(t *testing.T) {
	got := draftfloor.SplitGreetingLine("Greven, kurz und schmerzlos. Was ist der Stand?", "Greven")
	want := "Greven,\n\nkurz und schmerzlos. Was ist der Stand?"
	if got != want {
		t.Errorf("got %q, want the greeting on its own line", got)
	}
}

// The terser register ends the greeting on a full stop.
func TestAGreetingEndingInAFullStopIsSplit(t *testing.T) {
	got := draftfloor.SplitGreetingLine("Greven. Wir haben die Frage offen.", "Greven")
	want := "Greven.\n\nWir haben die Frage offen."
	if got != want {
		t.Errorf("got %q, want the greeting on its own line", got)
	}
}

// A greeting already on its own line is left exactly as it is.
//
// Rerunning the split must change nothing, or a second pass would keep adding
// blank lines to a draft that was already right.
func TestAGreetingAlreadyOnItsOwnLineIsUntouched(t *testing.T) {
	const body = "Greven,\n\nwir haben die Frage offen."
	if got := draftfloor.SplitGreetingLine(body, "Greven"); got != body {
		t.Errorf("got %q, want the body unchanged", got)
	}
}

// A body that does not open with the recipient's name is left alone.
//
// The repair is only safe where the split is unambiguous. A break guessed into
// the middle of a sentence is worse than the run-on it replaced.
func TestABodyNotOpeningOnTheNameIsUntouched(t *testing.T) {
	const body = "Kurz und schmerzlos: was ist der Stand bei Michael?"
	if got := draftfloor.SplitGreetingLine(body, "Greven"); got != body {
		t.Errorf("got %q, want the body unchanged", got)
	}
}

// The name has to be followed by a greeting separator, not more of a word.
//
// "Grevenstrasse" starts with "Greven" and is not a greeting. Splitting on the
// prefix alone would cut a word in half.
func TestANameThatIsOnlyAPrefixIsUntouched(t *testing.T) {
	const body = "Grevenstrasse 4 ist die neue Adresse, sag ich dir nur kurz."
	if got := draftfloor.SplitGreetingLine(body, "Greven"); got != body {
		t.Errorf("got %q, want the body unchanged", got)
	}
}

// With no recipient name there is nothing to anchor a split to.
func TestNoRecipientNameLeavesTheBodyAlone(t *testing.T) {
	const body = "Hallo, wir haben die Frage offen."
	if got := draftfloor.SplitGreetingLine(body, ""); got != body {
		t.Errorf("got %q, want the body unchanged", got)
	}
}

// The split moves a break and never a word.
//
// It runs over model-written prose, so the guarantee that makes it safe is
// that the words going out are still the model's. Compared with whitespace
// removed, the text before and after must be identical.
func TestTheSplitChangesNoWords(t *testing.T) {
	const body = "Greven, kurz und schmerzlos. Was ist der Stand bei Michael Grodd?"
	got := draftfloor.SplitGreetingLine(body, "Greven")
	if squeeze(got) != squeeze(body) {
		t.Errorf("the split rewrote the message: %q became %q", body, got)
	}
}

// A lowercased greeting is the same greeting, and the same run-on line.
//
// The body keeps the spelling the model chose: only the match is relaxed, so
// the repair still moves a break and rewrites nothing.
func TestALowercasedGreetingIsSplitAndKeepsItsSpelling(t *testing.T) {
	got := draftfloor.SplitGreetingLine("greven, kurz und schmerzlos. Was ist der Stand?", "Greven")
	want := "greven,\n\nkurz und schmerzlos. Was ist der Stand?"
	if got != want {
		t.Errorf("got %q, want the greeting split with the model's own capitalisation", got)
	}
}

// A name carrying non-ASCII letters splits at the right place.
//
// The separator is read as the byte after the name, and a multi-byte name
// would put that index inside a rune if the length were counted in characters
// rather than bytes.
func TestANameWithUmlautsIsSplit(t *testing.T) {
	got := draftfloor.SplitGreetingLine("Jörg, wie ist der Stand?", "Jörg")
	want := "Jörg,\n\nwie ist der Stand?"
	if got != want {
		t.Errorf("got %q, want the split to land after the name", got)
	}
}

// A name that already contains a full stop is not cut at its own title.
func TestANameContainingAFullStopSplitsAfterTheWholeName(t *testing.T) {
	got := draftfloor.SplitGreetingLine("Dr. Meier, wir haben die Frage offen.", "Dr. Meier")
	want := "Dr. Meier,\n\nwir haben die Frage offen."
	if got != want {
		t.Errorf("got %q, want the split after the whole name rather than inside it", got)
	}
}

// A FORMAL greeting opens on the surname, and is repaired too.
//
// Which name opens the message is the register's choice — the shared rules
// send the formal form to the surname — so a repair that knew only the first
// name left every formal draft running on.
func TestAFormalGreetingOnTheSurnameIsSplit(t *testing.T) {
	got := draftfloor.SplitGreetingLine("Greven, wir haben die Frage offen.", "Marcus", "Greven")
	want := "Greven,\n\nwir haben die Frage offen."
	if got != want {
		t.Errorf("got %q, want the formal greeting on its own line", got)
	}
}

// The first name is tried first, so a familiar greeting is unaffected by the
// surname also being offered.
func TestTheFirstNameStillWinsWhenBothAreOffered(t *testing.T) {
	got := draftfloor.SplitGreetingLine("Marcus, wir haben die Frage offen.", "Marcus", "Greven")
	want := "Marcus,\n\nwir haben die Frage offen."
	if got != want {
		t.Errorf("got %q, want the familiar greeting on its own line", got)
	}
}

// A body already broken with CRLF is left alone.
//
// Windows line endings are still line endings. Treating them as unbroken
// inserts a second break in front of the first and stacks blank lines.
func TestACRLFBodyIsRecognisedAsAlreadyBroken(t *testing.T) {
	const body = "Greven,\r\n\r\nwir haben die Frage offen."
	if got := draftfloor.SplitGreetingLine(body, "Greven"); got != body {
		t.Errorf("got %q, want the body unchanged", got)
	}
}

// squeeze removes every space and newline, leaving only the characters that
// carry meaning.
func squeeze(s string) string {
	out := make([]rune, 0, len(s))
	for _, r := range s {
		if r != ' ' && r != '\n' && r != '\t' {
			out = append(out, r)
		}
	}
	return string(out)
}
