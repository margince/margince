// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The name tests state their cases as raw names; production compares forms.

func bestCompanyNamePairing(candidateDisplay, candidateLegal, rowDisplay, rowLegal string) CompanyCandidateScore {
	return bestCompanyFormPairing(companyNamePairOf(candidateDisplay, candidateLegal), companyNamePairOf(rowDisplay, rowLegal))
}

func sharesADistinctiveWord(a, b string) bool {
	return formsShareADistinctiveWord(companyNameFormOf(a), companyNameFormOf(b))
}

func companyNamesAreTheSame(left, right string) bool {
	return foldedKeysAreTheSame(foldedNameKey(left), foldedNameKey(right))
}
