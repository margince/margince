// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// companyNameForm is one name with everything the pairing gate and the score
// read, derived once. One name is scored against many, and normalizing it again
// for every pair is where nearly all of that time went.
type companyNameForm struct {
	value string
	// matching is companyNameForMatching: what the gate and the score compare.
	matching string
	scored   string
	squashed string
	tokens   []string
	syllabic bool
	// folded is foldedNameKey, which decides ExactName.
	folded string
}

func companyNameFormOf(name string) companyNameForm {
	matching := companyNameForMatching(name)
	_, market := matchingFormOf(name)
	return companyNameForm{
		value:    name,
		matching: matching,
		scored:   normalizeName(matching),
		squashed: squashedCompanyName(name),
		tokens:   distinctiveCompanyTokens(name),
		syllabic: isSyllabicMarket(market),
		folded:   foldedNameKey(name),
	}
}

// companyNamePair is a company's two name axes, prepared.
type companyNamePair struct {
	display, legal companyNameForm
}

func companyNamePairOf(display, legal string) companyNamePair {
	return companyNamePair{display: companyNameFormOf(display), legal: companyNameFormOf(legal)}
}

// companyPairRival is the fuzzy tier's verdict on one incumbent's names: the
// score, and whether it clears the review threshold.
func companyPairRival(candidate, incumbent companyNamePair) (CompanyCandidateScore, bool) {
	score := bestCompanyFormPairing(candidate, incumbent)
	return score, score.Confidence >= dedupeReviewThreshold
}
