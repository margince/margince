// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package claims

import "github.com/margince/margince/backend/internal/shared/kernel/promptref"

// refPrefixes is the ref prefix of each record kind a sentence may cite. Ids
// of no citable kind read r1, r2, … and cite nothing.
var refPrefixes = map[string]string{
	"deal": "d", "activity": "a", "contact": "c", "lead": "l", "company": "co",
	"profile_field": "pf", "fact": "fa", "participant": "p", "document": "doc", "thread": "t",
	"": "r",
}

// ShowRefs writes each record id in payload as a ref of the kind known gives
// it. The request shows the result, and the parse reads the reply's
// citations back through it with ReadRefs.
func ShowRefs(payload string, known map[Evidence]string) *promptref.Payload {
	kindOf := make(map[string]string, len(known))
	for cited := range known {
		kindOf[cited.EntityID] = cited.EntityType
	}
	return ShowKinds(payload, kindOf)
}

// ShowKinds is ShowRefs for a site that keys its citable records by id alone,
// with the kind each one is cited as.
func ShowKinds(payload string, kindOf map[string]string) *promptref.Payload {
	return promptref.Rewrite(payload, kindOf, refPrefixes)
}

// ReadRefs reads each citation's ref back to the record id it was shown for.
// A ref never shown as the cited kind reads as no record, so Grounded drops
// its sentence. A sentence that writes a ref into its prose is dropped here:
// a ref in text is developer output, as an id is.
func ReadRefs(sentences []Sentence, shown *promptref.Payload) []Sentence {
	out := make([]Sentence, 0, len(sentences))
	for _, sentence := range sentences {
		if shown.Names(sentence.Text) {
			continue
		}
		evidence := make([]Evidence, 0, len(sentence.Evidence))
		for _, cited := range sentence.Evidence {
			cited.EntityID, _ = shown.ID(cited.EntityType, cited.EntityID)
			evidence = append(evidence, cited)
		}
		sentence.Evidence = evidence
		out = append(out, sentence)
	}
	return out
}
