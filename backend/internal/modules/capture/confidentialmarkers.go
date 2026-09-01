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
// Whole-word, so "NDA" does not fire on "AGENDA" and "vertraulich" does not
// fire inside a longer compound the sender did not mean as a marker. The
// German words carry their inflected endings because a subject reads
// "Vertrauliche Unterlagen" as readily as "[Vertraulich]".
// The German endings are the full set a subject line uses, not a sample: -en is
// the most common of them ("Vertrauliche Unterlagen" is nominative, "Zur
// vertraulichen Behandlung" dative), and each ending missing here is a message
// its sender explicitly marked that publishes to the workspace.
//
// No separate arm for "[Vertraulich]" or "streng vertraulich": a bracket and a
// space are both word boundaries, so the inflected arm already matches them.
var confidentialMarkers = regexp.MustCompile(
	`(?i)(\bvertraulich(e|en|em|er|es)?\b|\bvertraulichkeitsvereinbarung\b|` +
		`\bconfidential\b|\bprivileged\b|\bnon-disclosure\b|\bNDA\b)`)

// explicitlyConfidential answers whether a subject line carries a marker its
// sender meant as one.
func explicitlyConfidential(subject string) bool {
	if strings.TrimSpace(subject) == "" {
		return false
	}
	return confidentialMarkers.MatchString(subject)
}
