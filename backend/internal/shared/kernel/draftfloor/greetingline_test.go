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
