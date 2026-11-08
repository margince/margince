// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package companydossier

import (
	"github.com/margince/margince/backend/internal/compose/claims"
	"github.com/margince/margince/backend/internal/shared/kernel/promptref"
)

// growthFitRefs names the records the growth-fit prompt shows: profile fields
// p1, p2, … and facts f1, f2, …, in the order the summary lists them.
// GrowthFitRequest and ParseGrowthFit both build it from the same input.
func growthFitRefs(in Input) claims.Refs {
	fields, facts := promptref.New("p"), promptref.New("f")
	for _, field := range in.ProfileFields {
		if field.Id != nil {
			fields.For(field.Id.String())
		}
	}
	for _, fact := range in.Facts {
		if fact.Id != nil {
			facts.For(fact.Id.String())
		}
	}
	return claims.Refs{citeProfileField: fields, citeFact: facts}
}

// readRefs turns every citation of the reply into the record id its ref names,
// before the grounding filter judges it.
func (p growthFitClaims) readRefs(refs claims.Refs) growthFitClaims {
	p.PositiveFactors = refs.Read(p.PositiveFactors)
	p.NegativeFactors = refs.Read(p.NegativeFactors)
	p.Whitespace = refs.Read(p.Whitespace)
	p.Objections = refs.Read(p.Objections)
	if p.RecommendedAngle != nil {
		angle := refs.Read([]claims.Sentence{*p.RecommendedAngle})
		p.RecommendedAngle = nil
		if len(angle) == 1 {
			p.RecommendedAngle = &angle[0]
		}
	}
	subScores := make([]GrowthFitSubScore, 0, len(p.SubScores))
	for _, sub := range p.SubScores {
		if len(refs.Read([]claims.Sentence{{Text: sub.Reason}})) == 0 {
			continue
		}
		sub.Evidence = refs.ReadEvidence(sub.Evidence)
		subScores = append(subScores, sub)
	}
	p.SubScores = subScores
	return p
}
