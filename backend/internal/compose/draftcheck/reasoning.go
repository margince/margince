// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package draftcheck

// The reasoning channel: the chips a draft shows the rep as its provenance.

import (
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/convstate"
	"github.com/margince/margince/backend/internal/shared/kernel/textlang"
)

// directedRelationship are the ways a draft claims who introduced, referred or
// first contacted whom.
//
// The product holds no contact-to-contact referral record — referred_by is
// constrained company-to-company — so a directed introduction fact in a draft is
// necessarily read out of quoted correspondence, which is how the reported
// defect got the direction backwards. Silence about introductions is the
// correct behaviour today (DRAFT-AC-E-7), which makes this list a flat refusal
// rather than a judgement about which direction is right.
// The NOUN, not the preposition. A first attempt enumerated "introduction by",
// "introduced by" and the rest; the model wrote "introduction TO" and walked
// straight through. There is no honest use of these words in a chip while the
// product holds no referral record, so the word itself is the refusal and the
// grammar around it does not have to be predicted.
var directedRelationship = map[textlang.Lang][]string{
	textlang.English: {
		// Stems, matched as a word PREFIX, because the word form is not
		// predictable and enumerating it has failed twice on a live stack:
		// "introduction by" missed "introduction to", and the noun list missed
		// "introductory". "introduc" covers introduction/introduced/introducing/
		// introductory; "refer" covers referral/referred/referring.
		"introduc", "intro", "refer",
		"put us in touch", "connected us",
	},
	textlang.German: {
		"vorstell", "vorgestellt", "empfehl", "empfohlen",
		"vermittl", "vermittelt", "in kontakt gebracht",
	},
	textlang.Vietnamese: {
		"giới thiệu", "được giới thiệu",
	},
}

// Reasoning reads the labels a draft shows the rep as its provenance.
//
// A chip is the product explaining itself, and a rep reads it less critically
// than the body they are about to send — so a wrong one is worse there. It gets
// the same phrase lists as the body, plus the directed-relationship refusal,
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
	// on a German draft — "shared contact introduction" appeared under German
	// prose on a live stack, and a German-only list did not see it.
	for _, phrase := range allDirectedRelationshipPhrases() {
		if startsWord(lowered, phrase) {
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
	// than echoed from the counterparty or the caller.
	return append(findings, Body(label, lang, band, Grounds{})...)
}

// allDirectedRelationshipPhrases joins the lists of all languages, for the
// reasoning channel. A chip's own language is not the draft's.
func allDirectedRelationshipPhrases() []string {
	var out []string
	for _, phrases := range directedRelationship {
		out = append(out, phrases...)
	}
	return out
}
