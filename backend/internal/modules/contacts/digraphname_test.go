// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import "testing"

func TestTheDigraphKeyMeetsAnUmlautAndItsTransliteration(t *testing.T) {
	t.Parallel()

	cases := []struct {
		a, b string
		same bool
	}{
		{"Björn Beispiel", "Bjoern Beispiel", true},
		{"JÜRGEN MÜLLER", "Juergen Mueller", true},
		// NFD input: "o" followed by a combining diaeresis.
		{"Björn", "Bjoern", true},
		{"Bjoern", "Bjorn", false},
	}
	for _, c := range cases {
		if got := digraphNameKey(c.a) == digraphNameKey(c.b); got != c.same {
			t.Errorf("digraphNameKey(%q) == digraphNameKey(%q) is %v, want %v", c.a, c.b, got, c.same)
		}
	}
	if NormalizeContactName("Björn") != NormalizeContactName("Bjorn") {
		t.Error("the plain key no longer folds ö to o; it feeds locks and stored hashes and must not move")
	}
}

func TestTheContactScoreReadsAnUmlautAsItsDigraph(t *testing.T) {
	t.Parallel()

	if got := contactNameSimilarity("Björn Beispiel", "Bjoern Beispiel"); got != 1 {
		t.Errorf("contactNameSimilarity = %v, want 1", got)
	}
	if contactNameSimilarity("Björn Beispiel", "Bjoern Beispiel") <= nameSimilarity("Björn Beispiel", "Bjoern Beispiel") {
		t.Error("the contact score gains nothing from the digraph spelling")
	}
}
