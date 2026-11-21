// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// The ladder's answer about records a batch will create or change but has not
// written. A batch lands its rows one at a time, so the ladder meets each
// earlier row as a stored record. A test run writes nothing, so it asks here,
// and each answer applies the ladder's own scorers to the pair.

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
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

// CompanyLookAlike answers whether DedupeCompany, asked about c with no
// domains of its own, pairs it with held once held is written.
//
// c brings no phone and a create shares no contact, so the only corroboration
// left is a street address. TestTheLadderAndItsBatchHalfAgree holds the two
// answers together.
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

// ContactLookAlike answers whether DedupeContact, asked about c with no
// employer of its own, pairs it with held once held is written. It pairs on a
// shared address, or on the fuzzy tier's score with each employer domain.
func ContactLookAlike(c ContactCandidate, held NewContact) bool {
	for _, mine := range c.Emails {
		for _, theirs := range held.Emails {
			if normalizeEmail(mine) != "" && normalizeEmail(mine) == normalizeEmail(theirs) {
				return true
			}
		}
	}
	row := contactCandidateRow{fullName: held.FullName, mailDomains: mailDomainsOf(held.Emails)}
	best := contactConfidence(c, row)
	for _, domain := range held.EmployerDomains {
		row.companyDomain = &domain
		best = max(best, contactConfidence(c, row))
	}
	return best >= dedupeReviewThreshold
}

// Batch is the records a batch will create or change, in the state the ladder
// meets them. A stored record a row changes is answered from here, never from
// what the estate still holds.
type Batch struct {
	companies []NewCompany
	contacts  []NewContact
}

// AddCompany files a company a row leaves behind. A stored company written
// twice keeps the state its last row leaves.
func (b *Batch) AddCompany(c NewCompany) {
	if c.ID != (ids.UUID{}) {
		b.companies = slices.DeleteFunc(b.companies, func(earlier NewCompany) bool { return earlier.ID == c.ID })
	}
	b.companies = append(b.companies, c)
}

// AddContact files a contact as AddCompany files a company.
func (b *Batch) AddContact(c NewContact) {
	if c.ID != (ids.UUID{}) {
		b.contacts = slices.DeleteFunc(b.contacts, func(earlier NewContact) bool { return earlier.ID == c.ID })
	}
	b.contacts = append(b.contacts, c)
}

// Changes reports whether a row of the batch changes the stored record id.
func (b *Batch) Changes(id ids.UUID) bool {
	if id == (ids.UUID{}) {
		return false
	}
	return slices.ContainsFunc(b.companies, func(c NewCompany) bool { return c.ID == id }) ||
		slices.ContainsFunc(b.contacts, func(c NewContact) bool { return c.ID == id })
}

// CompanyLookAlike answers whether the ladder pairs c with a company of the batch.
func (b *Batch) CompanyLookAlike(c NewCompany) bool {
	return slices.ContainsFunc(b.companies, func(held NewCompany) bool { return CompanyLookAlike(c, held) })
}

// ContactLookAlike answers whether the ladder pairs c with a contact of the batch.
func (b *Batch) ContactLookAlike(c ContactCandidate) bool {
	return slices.ContainsFunc(b.contacts, func(held NewContact) bool { return ContactLookAlike(c, held) })
}

// Company answers the first company of the batch that wanted accepts.
func (b *Batch) Company(wanted func(NewCompany) bool) (NewCompany, bool) {
	at := slices.IndexFunc(b.companies, wanted)
	if at < 0 {
		return NewCompany{}, false
	}
	return b.companies[at], true
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

// CompanyAfterUpdate is the stored company current as an update leaves it. An
// empty name or a nil address keeps the stored one, and domain joins the rest.
func CompanyAfterUpdate(id ids.UUID, current []byte, displayName, legalName, domain string, address *crmcontracts.Address) (NewCompany, error) {
	var stored crmcontracts.Company
	if err := json.Unmarshal(current, &stored); err != nil {
		return NewCompany{}, fmt.Errorf("contacts: reading the stored company: %w", err)
	}
	held := NewCompany{ID: id, DisplayName: cmp.Or(displayName, stored.DisplayName),
		LegalName: cmp.Or(legalName, derefString(stored.LegalName)), Address: cmp.Or(address, stored.Address), Domains: []string{domain}}
	if stored.Domains != nil {
		for _, domain := range *stored.Domains {
			held.Domains = append(held.Domains, domain.Domain)
		}
	}
	return held, nil
}

// ContactAfterUpdate is the stored contact current as UpdateContact leaves it
// with in, employed where it is now. The import merges addresses, never drops one.
func ContactAfterUpdate(ctx context.Context, tx pgx.Tx, id ids.ContactID, current []byte, in UpdateContactInput) (NewContact, error) {
	var stored crmcontracts.Contact
	if err := json.Unmarshal(current, &stored); err != nil {
		return NewContact{}, fmt.Errorf("contacts: reading the stored contact: %w", err)
	}
	held := NewContact{ID: id.UUID, FullName: stored.FullName}
	if name := displayNameAfter(stored, in); name != nil {
		held.FullName = *name
	}
	if stored.Emails != nil {
		for _, email := range *stored.Emails {
			held.Emails = append(held.Emails, string(email.Email))
		}
	}
	for _, email := range in.Emails {
		held.Emails = append(held.Emails, email.Email)
	}
	var err error
	held.EmployerDomains, err = employerDomainsTx(ctx, tx, id)
	return held, err
}

// DomainNames are the domains the preview's company is created with.
func (p ContactEmployerPreview) DomainNames() []string {
	names := make([]string, 0, len(p.Domains))
	for _, domain := range p.Domains {
		names = append(names, domain.Domain)
	}
	return names
}

// employerDomainsTx answers the live domains of the contact's current primary
// employer, as the contact ladder joins them.
func employerDomainsTx(ctx context.Context, tx pgx.Tx, contact ids.ContactID) ([]string, error) {
	return domainsQuery(ctx, tx, `
		SELECT od.domain FROM relationship r
		  JOIN company_domain od ON od.company_id = r.company_id AND od.archived_at IS NULL
		 WHERE r.contact_id = $1 AND r.kind = 'employment' AND r.archived_at IS NULL
		   AND `+employment.CurrentPrimarySQL("r")+` ORDER BY od.domain`, contact.UUID)
}

func companyDomainsTx(ctx context.Context, tx pgx.Tx, company ids.CompanyID) ([]string, error) {
	return domainsQuery(ctx, tx, `
		SELECT domain FROM company_domain WHERE company_id = $1 AND archived_at IS NULL ORDER BY domain`, company.UUID)
}

func domainsQuery(ctx context.Context, tx pgx.Tx, query string, id ids.UUID) ([]string, error) {
	rows, err := tx.Query(ctx, query, id)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading an employer's domains: %w", err)
	}
	domains, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("contacts: reading an employer's domains: %w", err)
	}
	return domains, nil
}
