// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcheck

// The reasoning channel: the chips a draft shows the rep as its provenance.

import (
	"slices"
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// introductionEvent are the chip labels that say an introduction HAPPENED,
// beyond the directed claims a body is also refused (directedIntroduction).
//
// The product holds no contact-to-contact referral record — referred_by is
// constrained company-to-company — so an introduction named as an event was
// read out of quoted correspondence, which is how the reported defect got the
// direction backwards.
//
// Every entry names the event, never the act of writing. A bare stem refuses
// "Mich kurz vorstellen" and "introduce company and offer call" — the rep's own
// purpose, which the prompt asks the body to state — and a retry spent on that
// drops the sender's name from a good body.
var introductionEvent = map[textlang.Lang][]string{
	textlang.English: {
		"introduction to", "introduced to", "introducing us", "introducing me to",
		"introductory connection", "previous introduction", "earlier introduction",
		"prior introduction", "contact introduction", "introduction made",
		"intro by", "intro from", "intro via", "intro to", "intro made",
		"referral", "referrals", "referred",
	},
	textlang.German: {
		"vermittelt", "vermittlung", "nach intro", "nach dem intro",
	},
	textlang.Vietnamese: {
		"được giới thiệu",
	},
}

// Reasoning reads the labels a draft shows the rep as its provenance.
//
// A chip is the product explaining itself, and a rep reads it less critically
// than the body they are about to send — so a wrong one is worse there. It gets
// the same phrase lists as the body, plus the introduction-event refusal,
// which caught "Follow-up to previous introduction by Romina Medici" on a
// thread where the other party made the introduction.
//
// Nothing here is gated on the band: a chip is not prose, and "as discussed" in
// a label is a claim about the record rather than a turn of phrase.
func Reasoning(labels []string, lang textlang.Lang, band convstate.Band) []Finding {
	var findings []Finding
	for _, label := range labels {
		findings = append(findings, labelFindings(label, lang, band)...)
	}
	for i := range findings {
		findings[i].InLabel = true
	}
	return findings
}

// labelFindings is what is wrong with one reasoning chip.
func labelFindings(label string, lang textlang.Lang, band convstate.Band) []Finding {
	var findings []Finding
	lowered := strings.ToLower(label)
	// EVERY language, not just the draft's. A chip is written for the rep
	// rather than the recipient, and the model reaches for English there even
	// on a German draft ("shared contact introduction" under German prose),
	// which a German-only list does not see.
	for _, phrase := range allIntroductionPhrases() {
		if contains(lowered, phrase) {
			findings = append(findings, Finding{
				Rule:   RuleInventedRelationship,
				Phrase: phrase,
				Why: "no referral record exists to support who introduced whom, so a " +
					"chip asserting one states a fact the product does not hold",
			})
		}
	}
	// A chip is checked as an unthreaded body with nothing booked or met,
	// whatever the draft beside it is: it is the product's own claim about what
	// it wrote from, so a call or a date named there is asserted by us rather
	// than echoed from the counterparty or the caller. The body's own
	// introduction rule is the draft language's subset of the pass above.
	return append(findings, slices.DeleteFunc(Body(label, lang, band, Grounds{}), func(f Finding) bool {
		return f.Rule == RuleInventedRelationship
	})...)
}

// allIntroductionPhrases joins every language's directed and event lists, for
// the reasoning channel. A chip's own language is not the draft's.
func allIntroductionPhrases() []string {
	var out []string
	for _, lists := range []map[textlang.Lang][]string{directedIntroduction, introductionEvent} {
		for _, phrases := range lists {
			out = append(out, phrases...)
		}
	}
	return out
}
