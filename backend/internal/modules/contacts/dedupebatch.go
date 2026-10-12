// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The ladder's answer about records a batch will create but has not written.
// A batch writer lands its rows one at a time, so its commit meets each earlier
// row as an incumbent. A dry run writes nothing and carries those rows itself.
// These sets score them with the ladder's own per-row functions. The query's
// trigram arms only narrow what gets scored, so they are not repeated.

import "slices"

// CompanyBatch is the companies a batch will create, in the order it decided.
type CompanyBatch struct {
	members []batchCompany
}

type batchCompany struct {
	names   companyNamePair
	domains []string
}

// Add records a company the batch will create.
func (b *CompanyBatch) Add(c CompanyCandidate) {
	domains := make([]string, 0, len(c.Domains))
	for _, d := range c.Domains {
		domains = append(domains, normalizeDomain(d))
	}
	b.members = append(b.members, batchCompany{names: companyNamePairOf(c.DisplayName, c.LegalName), domains: domains})
}

// HasRival answers whether DedupeCompany would name an earlier member as a
// duplicate of c, had that member been written. Either the fuzzy tier scores
// the names over the threshold, or c's display name spells a member's domain.
func (b *CompanyBatch) HasRival(c CompanyCandidate) bool {
	if NormalizeCompanyName(c.DisplayName) == "" && NormalizeCompanyName(c.LegalName) == "" {
		return false
	}
	candidate := companyNamePairOf(c.DisplayName, c.LegalName)
	spelled, spellsDomain := displayNameDomain(c.DisplayName)
	for _, held := range b.members {
		if spellsDomain && slices.Contains(held.domains, spelled) {
			return true
		}
		if _, rival := companyPairRival(candidate, held.names); rival {
			return true
		}
	}
	return false
}

// ContactBatch is the contacts a batch will create, filed under the mail
// domains of their addresses.
//
// Filed by domain because a batch row carries no employer. A shared mail
// domain is then the only employer evidence between two rows, and a name alone
// cannot clear the threshold.
type ContactBatch struct {
	namesByDomain map[string][]string
}

// Add records a contact the batch will create. A consumer mail domain is not
// filed: the ladder never reads one as a shared employer.
func (b *ContactBatch) Add(c ContactCandidate) {
	if b.namesByDomain == nil {
		b.namesByDomain = map[string][]string{}
	}
	for _, e := range c.Emails {
		if domain := emailDomain(e); domain != "" && sharedEmployerDomain(c, domain) {
			b.namesByDomain[domain] = append(b.namesByDomain[domain], c.FullName)
		}
	}
}

// HasRival answers whether the fuzzy tier of DedupeContact would name an
// earlier member as a duplicate of c, had that member been written. An address
// two rows share is the exact tier's question, which the batch asks first.
func (b *ContactBatch) HasRival(c ContactCandidate) bool {
	if NormalizeContactName(c.FullName) == "" {
		return false
	}
	for _, e := range c.Emails {
		domain := emailDomain(e)
		for _, name := range b.namesByDomain[domain] {
			row := contactCandidateRow{fullName: name, mailDomains: []string{domain}}
			if contactConfidence(c, row) >= dedupeReviewThreshold {
				return true
			}
		}
	}
	return false
}
