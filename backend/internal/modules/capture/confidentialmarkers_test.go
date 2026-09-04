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
		{"Vertraulichkeitsvereinbarung", true, "the German word for an NDA — and a sender typing it has written \"vertraulich\" at the front of it, which is the marking this list is for"},
		{"Confidential: Q3 numbers", true, "the English marker"},

		// Naming an agreement is not asking for confidence. An NDA is a routine
		// contract between two COMPANIES, signed by the company rather than by a
		// person, and that one exists is not itself a secret. These are threads
		// ABOUT ordinary contracting, and holding them took a deal's own
		// paperwork away from the team doing the deal.
		{"NDA for review", false, "the abbreviation names a document and asks for nothing"},
		{"Non-disclosure agreement", false, "the same document, written out"},
		{"Re: NDA", false, "the shape a real thread takes"},
		{"NDA signed, we can proceed", false, "ordinary deal progress, which is what a rep needs their team to see"},

		{"AGENDA for Monday", false, "never a marker even while NDA was one: the match is whole-word"},
		{"Quarterly report", false, "ordinary mail"},
		{"", false, "no subject at all"},
	} {
		if got := explicitlyConfidential(c.subject); got != c.marked {
			t.Errorf("%q: marked=%v, want %v — %s", c.subject, got, c.marked, c.why)
		}
	}
}
