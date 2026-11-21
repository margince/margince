// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"github.com/margince/margince/backend/internal/platform/namematch"
)

// companyNameForms is one name in every form the pairing compares, worked out
// once. A batch scores one name against many, so it keeps these per record.
type companyNameForms struct {
	matching, squashed, sameKey string
	folded                      []rune
	tokens                      []string
	syllabic                    bool
}

func companyNameFormsOf(name string) companyNameForms {
	matching, market := matchingFormOf(name)
	return companyNameForms{
		matching: matching, folded: namematch.ScoringRunes(namematch.Fold(matching)),
		squashed: squashedOf(matching),
		sameKey:  foldedNameKey(name), tokens: distinctiveTokensOf(matching), syllabic: isSyllabicMarket(market),
	}
}

// companyNamePair is a company's two name axes in their forms.
type companyNamePair struct {
	display, legal         string
	displayForm, legalForm companyNameForms
}

func companyNamePairOf(display, legal string) companyNamePair {
	return companyNamePair{display: display, legal: legal, displayForm: companyNameFormsOf(display), legalForm: companyNameFormsOf(legal)}
}

// bestCompanyNamePairing scores every pairing of the two name axes and reports
// which one won, with the two values compared. An empty axis scores nothing.
// A pairing scores only once the names pass the distinctive-word gate
// (companynamegate.go). The gate and the score read one matching form.
func bestCompanyNamePairing(candidateDisplay, candidateLegal, rowDisplay, rowLegal string) CompanyCandidateScore {
	return bestFormsPairing(companyNamePairOf(candidateDisplay, candidateLegal), companyNamePairOf(rowDisplay, rowLegal))
}

// bestFormsPairing is bestCompanyNamePairing over names in their forms.
func bestFormsPairing(candidate, row companyNamePair) CompanyCandidateScore {
	sides := []struct {
		value string
		form  companyNameForms
		field string
	}{{row.display, row.displayForm, fieldDisplayName}, {row.legal, row.legalForm, fieldLegalName}}
	var best CompanyCandidateScore
	var exact, sameBrand bool
	for _, left := range []struct {
		value string
		form  companyNameForms
	}{{candidate.display, candidate.displayForm}, {candidate.legal, candidate.legalForm}} {
		for _, right := range sides {
			if left.form.matching == "" || right.form.matching == "" {
				continue
			}
			// The gate comes before the score: a pair with no word in common
			// has said nothing about being one company.
			if !formsShareADistinctiveWord(left.form, right.form) {
				continue
			}
			// Asked of every eligible pairing: the winner below is chosen on
			// score alone, and a tie would otherwise hide the exact pairing.
			exact = exact || sameNameKey(left.form.sameKey, right.form.sameKey)
			sameBrand = sameBrand || (left.form.squashed != "" && left.form.squashed == right.form.squashed)
			if score := namematch.JaroWinklerRunes(left.form.folded, right.form.folded); score > best.Confidence {
				best = CompanyCandidateScore{Confidence: score, MatchedField: right.field, CandidateValue: left.value, IncumbentValue: right.value}
			}
		}
	}
	best.ExactName, best.SameBrand = exact, sameBrand
	return best
}
