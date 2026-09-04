// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

// A sender saying, in the subject line, that a message is confidential.
//
// This is the one confidentiality signal that needs no model: the person who
// wrote the mail already told us. It runs inside the capture transaction, so a
// message marked [Vertraulich] is never workspace-readable for the window a
// classifier would take to reach the same conclusion.
//
// Deliberately narrow. Every word here is one a sender chose in order to mark a
// message, in a subject line, in a language the DACH market writes business mail
// in. Guessing from body text or from a topic is the classifier's job, and a
// wider list here would hold ordinary mail on a coincidence — "confidential" in
// a marketing subject about a data-protection webinar is not a confidential
// message.

import (
	"regexp"
	"strings"
)

// confidentialMarkers are the whole-word subject markers that hold a message.
//
// Every word here is one a sender writes IN ORDER TO MARK the message. That is
// the test this list is kept to, and it is why the names of agreements are not
// on it.
//
// "NDA" and "non-disclosure" were, and they are the wrong kind of word. An NDA
// is a routine agreement between two COMPANIES — signed by the company rather
// than by a person — and that one exists is not itself a secret. "Re: NDA",
// "NDA signed" and "NDA für das Projekt" are threads ABOUT ordinary
// contracting, so holding them took a deal's own paperwork away from the team
// doing the deal. Naming a document is not asking for confidence: the material
// the document covers may well be, and a sender who wants that held writes one
// of the words below.
//
// Whole-word, so "vertraulich" does not fire inside a longer compound the
// sender did not mean as a marker. The
// German words carry their inflected endings because a subject reads
// "Vertrauliche Unterlagen" as readily as "[Vertraulich]".
// The German endings are the full set a subject line uses, not a sample: -en is
// the most common of them ("Vertrauliche Unterlagen" is nominative, "Zur
// vertraulichen Behandlung" dative), and each ending missing here is a message
// its sender explicitly marked that publishes to the workspace.
//
// "Vertraulichkeitsvereinbarung" is the German for an NDA and stays, which is
// not the exception it looks like: a sender typing it has written "vertraulich"
// at the front of the word, and it is what a DACH sender reaches for when
// marking. The English abbreviation carries no such marking sense.
//
// No separate arm for "[Vertraulich]" or "streng vertraulich": a bracket and a
// space are both word boundaries, so the inflected arm already matches them.
var confidentialMarkers = regexp.MustCompile(
	`(?i)(\bvertraulich(e|en|em|er|es)?\b|\bvertraulichkeitsvereinbarung\b|` +
		`\bconfidential\b|\bprivileged\b)`)

// explicitlyConfidential answers whether a subject line carries a marker its
// sender meant as one.
func explicitlyConfidential(subject string) bool {
	if strings.TrimSpace(subject) == "" {
		return false
	}
	return confidentialMarkers.MatchString(subject)
}
