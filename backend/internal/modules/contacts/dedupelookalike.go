// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The ladder's answer about a record a batch will create but has not written.
// A batch lands its rows one at a time, so the ladder meets each earlier row
// as a stored record. A test run writes nothing, so it asks here, and each
// answer applies the ladder's own scorers to the pair.

import (
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// NewCompany is what a batch row says about a company it will create: the
// facts such a company holds the moment it is written.
type NewCompany struct {
	DisplayName, LegalName string
	Domains                []string
	Address                *crmcontracts.Address
}

// CompanyLookAlike answers whether DedupeCompany, asked about c with no
// domains of its own, pairs it with held once held is written.
//
// A new company shares no contact with anything, and c brings no phone, so
// the only corroboration left is a street address. TestTheLadderAndItsBatchHalfAgree
// holds the two answers together.
func CompanyLookAlike(c, held NewCompany) bool {
	if spelled, ok := displayNameDomain(c.DisplayName); ok {
		for _, domain := range held.Domains {
			if normalizeDomain(domain) == normalizeDomain(spelled) {
				return true
			}
		}
	}
	score := bestCompanyNamePairing(c.DisplayName, c.LegalName, held.DisplayName, held.LegalName)
	if score.Confidence < dedupeReviewThreshold {
		return false
	}
	if score.SameBrand {
		return true
	}
	address := func(company NewCompany) string {
		return candidateEvidence(CompanyCandidate{Address: company.Address}).addressKey
	}
	return address(c) != "" && address(c) == address(held)
}

// NewContact is what a batch row says about a contact it will create.
type NewContact struct {
	FullName string
	Emails   []string
}

// ContactLookAlike answers whether DedupeContact, asked about c, pairs it with
// held once held is written. Neither side has an employer yet. A pair files
// on a shared address, or on the fuzzy tier's score.
func ContactLookAlike(c ContactCandidate, held NewContact) bool {
	for _, mine := range c.Emails {
		for _, theirs := range held.Emails {
			if normalizeEmail(mine) != "" && normalizeEmail(mine) == normalizeEmail(theirs) {
				return true
			}
		}
	}
	row := contactCandidateRow{fullName: held.FullName, mailDomains: mailDomainsOf(held.Emails)}
	return contactConfidence(c, row) >= dedupeReviewThreshold
}
