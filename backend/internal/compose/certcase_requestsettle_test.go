// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "testing"

// The thing owed may be named in more than one way, and each names it.
//
// The check was one lowered substring. An Angebot asked for in a German thread
// came back as "Angebot für die zweite Charge schicken" from one model, as
// "Send offer for the second batch" from another and as "Send the quote" from a
// third — three honest names for one document — and whichever single token the
// scenario pinned marked the other two wrong. `remaining` is our own note to
// ourselves, and the models write it in the thread's language as readily as the
// installation's, so pinning one token pinned a language with it.
//
// This is not the "accept either finding" relaxation that account_scan did NOT
// want: there the two answers were different FINDINGS and picking between them
// was the thing under test. Here they are the same finding under different
// words.
func TestRemainingMayBeNamedInSeveralRenderings(t *testing.T) {
	t.Parallel()
	const accepted = "angebot|offer|quote"

	for name, tc := range map[string]struct {
		remaining string
		want      bool
	}{
		"the German noun":        {"Angebot für die zweite Charge schicken", true},
		"an English translation": {"Send offer for the second batch", true},
		"the other translation":  {"Send the quote", true},
		"case does not matter":   {"SEND THE QUOTE", true},
		"empty names nothing":    {"", false},
		"a different thing":      {"Confirm the November dates", false},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := remainingNamesIt(tc.remaining, accepted); got != tc.want {
				t.Errorf("remainingNamesIt(%q, %q) = %v, want %v", tc.remaining, accepted, got, tc.want)
			}
		})
	}
}

// A single rendering still means that one, so the loosening reaches only the
// scenarios that ask for it.
func TestASingleRenderingIsStillDemanded(t *testing.T) {
	t.Parallel()
	if remainingNamesIt("Send the quote", "processing") {
		t.Error("a phrase naming something else satisfied a single-rendering expectation")
	}
	if !remainingNamesIt("Send the data-processing agreement", "processing") {
		t.Error("a phrase naming the expected thing was refused")
	}
}
