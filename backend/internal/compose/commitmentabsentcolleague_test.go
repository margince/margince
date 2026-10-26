// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import "testing"

func TestAColleagueIsNamedByTheirWrittenName(t *testing.T) {
	t.Parallel()
	for sentence, want := range map[string]bool{
		"Demo on the 5th with Robin.":          true,
		"Robin Weber sends it by Friday.":      true,
		"Demo with robin.":                     false,
		"Robinson sends it by Friday.":         false,
		"Demo on the 5th with Rob.":            false,
		"Mail Robin's notes to Ines on Friday": true,
	} {
		if got := namesColleague(sentenceWords(sentence), "Robin Weber", ""); got != want {
			t.Errorf("%q names Robin Weber: %v, want %v", sentence, got, want)
		}
	}
	if !namesColleague(sentenceWords("Danke, Lena macht das."), "Magdalena Huber", "Lena") {
		t.Error("the chosen greeting name is not read as the colleague's")
	}
}

func TestAWriterSpeakingOfThemselvesIsTheSubject(t *testing.T) {
	t.Parallel()
	for sentence, want := range map[string]bool{
		"I'll send it with Robin.":          true,
		"I’ll send it with Robin.":          true,
		"We send it by Friday.":             true,
		"Wir schicken es bis Freitag.":      true,
		"Demo with Robin on the 5th.":       false,
		"Imagine the demo on the 5th.":      false,
		"Let's book the demo with Robin.":   true,
		"Robin will send me the pricing.":   false,
		"Robin schickt mir die Unterlagen.": false,
		"Our team sends it by Friday.":      false,
	} {
		if got := writerIsTheSubject(sentence); got != want {
			t.Errorf("%q has the writer as the subject: %v, want %v", sentence, got, want)
		}
	}
}
