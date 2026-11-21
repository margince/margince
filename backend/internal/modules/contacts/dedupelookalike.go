// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The ladder's answer about records a batch will create or change but has not
// written. A batch lands its rows one at a time, so the ladder meets each
// earlier row as a stored record. A test run writes nothing, so it asks here.
// Each answer runs the ladder's own scorers and filing rules on the pair.

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// NewCompany is a company as a batch row leaves it: the facts the ladder reads
// once the row is written. ID names a stored company the row changes, and Ref
// is the row's own key in the batch.
type NewCompany struct {
	ID                          ids.UUID
	Ref, DisplayName, LegalName string
	Domains                     []string
	Address                     *crmcontracts.Address
}

// NewContact is a contact as a batch row leaves it. EmployerDomains are the
// domains of the company the contact is employed at by then.
type NewContact struct {
	ID              ids.UUID
	FullName        string
	Emails          []string
	EmployerDomains []string
}

// InTx runs fn in a transaction of the caller's workspace.
type InTx func(fn func(pgx.Tx) error) error

// CompanyLookAlike answers whether DedupeCompany, asked about c with no domains
// of its own, pairs it with held once held is written. c brings no phone and a
// create shares no contact, so a street address is the only fact to share.
func CompanyLookAlike(ctx context.Context, inTx InTx, c, held NewCompany) (bool, error) {
	var batch Batch
	batch.AddCompany(held)
	return batch.CompanyLookAlike(ctx, inTx, c)
}

// ContactLookAlike answers whether DedupeContact pairs c with held once held is
// written. c is asked by name and address, with no employer and no name
// collisions, as an import row asks.
func ContactLookAlike(ctx context.Context, inTx InTx, c NewContact, consumerMail *freemail.Matcher, held NewContact) (bool, error) {
	var batch Batch
	batch.AddContact(held)
	return batch.ContactLookAlike(ctx, inTx, c, consumerMail)
}

// heldCompany is a company with the forms its comparisons read, worked out once.
type heldCompany struct {
	record  NewCompany
	names   companyNamePair
	street  companyEvidence
	spelled string
	domains []string
}

func heldCompanyOf(c NewCompany) heldCompany {
	held := heldCompany{
		record: c, names: companyNamePairOf(c.DisplayName, c.LegalName), street: candidateEvidence(CompanyCandidate{Address: c.Address}),
	}
	if spelled, ok := displayNameDomain(c.DisplayName); ok {
		held.spelled = normalizeDomain(spelled)
	}
	for _, domain := range c.Domains {
		if folded := normalizeDomain(domain); folded != "" {
			held.domains = append(held.domains, folded)
		}
	}
	return held
}

// verdict is the ladder's answer on the two before its candidate query. The
// asked name spells a domain held holds (nameDomainClaimant), or the pair
// scores and files (fuzzyCompany, keepCorroborated). A scored pair still needs
// the query to return held for scoring.
func (asked heldCompany) verdict(held heldCompany) (spelled, scored bool) {
	if asked.spelled != "" && slices.Contains(held.domains, asked.spelled) {
		return true, false
	}
	score := bestFormsPairing(asked.names, held.names)
	return false, score.Confidence >= dedupeReviewThreshold && filesOnName(score, sameStreet(asked.street, held.street))
}

// askedContact is a contact row asking, with its forms worked out once.
type askedContact struct {
	candidate ContactCandidate
	forms     contactNameForms
	lookups   []string
}

func askedContactOf(c NewContact, consumerMail *freemail.Matcher) askedContact {
	return askedContact{
		candidate: ContactCandidate{FullName: c.FullName, Emails: c.Emails, ConsumerMail: consumerMail},
		forms:     contactNameFormsOf(c.FullName), lookups: emailLookupKeys(c.Emails),
	}
}

// heldContact is a held contact as the candidate query returns it, with its
// stored addresses and name forms worked out once.
type heldContact struct {
	record          NewContact
	forms           contactNameForms
	row             contactCandidateRow
	stored          []string
	employerDomains []string
	// domainKey names the domains the employer agreement reads.
	domainKey string
}

func heldContactOf(c NewContact) heldContact {
	held := heldContact{
		record: c, forms: contactNameFormsOf(c.FullName), employerDomains: c.EmployerDomains,
		row: contactCandidateRow{fullName: c.FullName, mailDomains: mailDomainsOf(c.Emails)},
	}
	for _, email := range c.Emails {
		held.stored = append(held.stored, storedEmail(email))
	}
	held.domainKey = strings.Join(held.row.mailDomains, " ") + "|" + strings.Join(c.EmployerDomains, " ")
	return held
}

// verdict is the ladder's answer on the two before its candidate query. That is
// the exact email lane, or the fuzzy tier's score at held's best employer
// agreement. A scored pair still needs the query to return held for scoring.
// agreements keeps that agreement per set of held domains, which repeats.
func (asked askedContact) verdict(held heldContact, agreements map[string]float64) (exact, scored bool) {
	if slices.ContainsFunc(held.stored, func(stored string) bool { return slices.Contains(asked.lookups, stored) }) {
		return true, false
	}
	agreement, known := agreements[held.domainKey]
	if !known {
		row := held.row
		agreement = companyMatch(asked.candidate, row)
		for _, domain := range held.employerDomains {
			row.companyDomain = &domain
			agreement = max(agreement, companyMatch(asked.candidate, row))
		}
		agreements[held.domainKey] = agreement
	}
	return false, contactConfidenceReaches(asked.forms, held.forms, agreement, dedupeReviewThreshold)
}

// Without is the match with the companies skip names left out.
func (m CompanyMatch) Without(skip func(ids.UUID) bool) CompanyMatch {
	kept := slices.DeleteFunc(slices.Clone(m.Ranked), func(r CompanyCandidateScore) bool { return skip(r.CompanyID.UUID) })
	switch {
	case len(m.Ranked) == 0 && !skip(m.CompanyID.UUID):
		return m
	case len(kept) == 0:
		return CompanyMatch{Decision: DecisionNoMatch}
	}
	m.Ranked, m.CompanyID, m.Confidence = kept, kept[0].CompanyID, kept[0].Confidence
	return m
}

// Without is the resolution with the contacts skip names left out.
func (m ContactResolution) Without(skip func(ids.UUID) bool) ContactResolution {
	if len(m.Reviews) == 0 && !skip(m.ContactID.UUID) {
		return m
	}
	return resolutionOf(slices.DeleteFunc(slices.Clone(m.Reviews), func(r ReviewMatch) bool { return skip(r.ContactID.UUID) }))
}

// AnyCompanyMatchVisible reports whether the caller may see one of the
// companies the ladder matched. Every one is asked, not only the winner: the
// winner is ranked by score and id, not by who may read it. No early exit
// either, or the number of queries would tell where the visible one sits.
func AnyCompanyMatchVisible(ctx context.Context, tx pgx.Tx, m CompanyMatch) (bool, error) {
	var visible bool
	for _, candidate := range candidatesOf(m) {
		seen, err := auth.VisibleTo(ctx, tx, entityCompany, candidate.UUID)
		if err != nil {
			return false, fmt.Errorf("contacts: probing a matched company's visibility: %w", err)
		}
		visible = visible || seen
	}
	return visible, nil
}

// candidatesOf answers every company the ladder matched, best first. An exact
// (domain) collision has no ranked set and carries its answer in CompanyID.
func candidatesOf(match CompanyMatch) []ids.CompanyID {
	if len(match.Ranked) == 0 {
		return []ids.CompanyID{match.CompanyID}
	}
	out := make([]ids.CompanyID, 0, len(match.Ranked))
	for _, scored := range match.Ranked {
		out = append(out, scored.CompanyID)
	}
	return out
}
