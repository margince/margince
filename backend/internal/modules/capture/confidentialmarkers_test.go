// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import "testing"

// The marker vocabulary as a spec: which subject lines a sender meant as a
// confidentiality marker, and which merely contain a similar word.
//
// Every miss here is a message its sender explicitly marked that publishes to
// the workspace, so the German inflections are enumerated rather than sampled.
// Every false positive is ordinary mail held for a coincidence, which is why
// the match is whole-word and the list is short.
func TestASubjectMarkerIsRecognisedInTheFormsSendersActuallyWrite(t *testing.T) {
	for _, c := range []struct {
		subject string
		marked  bool
		why     string
	}{
		{"[Vertraulich] Aufhebungsvertrag", true, "the bracketed form, which needs no arm of its own — a bracket is a word boundary"},
		{"Streng vertraulich", true, "the intensified form, likewise"},
		{"Vertrauliche Unterlagen", true, "nominative"},
		{"Vertraulichen Unterlagen im Anhang", true, "accusative, the most common ending in a subject"},
		{"Vertraulichem Schreiben", true, "dative"},
		{"Zur vertraulichen Behandlung", true, "the phrase a German business letter actually uses"},
		{"Vertraulichkeitsvereinbarung", true, "the German word for an NDA"},
		{"Non-disclosure agreement", true, "its English name, which NDA alone would miss"},
		{"NDA for review", true, "the abbreviation, whole-word"},
		{"Confidential: Q3 numbers", true, "the English marker"},

		{"AGENDA for Monday", false, "NDA inside a longer word is not a marker"},
		{"Quarterly report", false, "ordinary mail"},
		{"", false, "no subject at all"},
	} {
		if got := explicitlyConfidential(c.subject); got != c.marked {
			t.Errorf("%q: marked=%v, want %v — %s", c.subject, got, c.marked, c.why)
		}
	}
}
