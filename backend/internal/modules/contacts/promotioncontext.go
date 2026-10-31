// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/diffhash"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// promotionContextFormat versions the digest, so a change to the facts it
// covers refuses approvals staged under the old shape instead of matching them.
const promotionContextFormat = "employer.v1"

// employerDomainFacts are what an address says about a company to resolve,
// read without asking who may see that company.
type employerDomainFacts struct {
	Host       string             `json:"host"`
	Domain     string             `json:"domain"`
	Consumer   bool               `json:"consumer"`
	Suppressed bool               `json:"suppressed"`
	Owners     []domainOwnerFacts `json:"owners"`
}

// domainOwnerFacts carry the owner because visibility follows it: a new owner
// can hide the company from the promoter without any domain changing.
type domainOwnerFacts struct {
	CompanyID ids.UUID  `json:"company_id"`
	OwnerID   *ids.UUID `json:"owner_id"`
}

func (s *Store) employerDomainFactsTx(ctx context.Context, tx pgx.Tx, email string) (employerDomainFacts, error) {
	matcher, err := s.consumerMailMatcher(ctx, tx)
	if err != nil {
		return employerDomainFacts{}, err
	}
	facts := employerDomainFacts{Host: owndomain.OfAddress(email)}
	domain, ok := freemail.Hostname(facts.Host)
	if !ok {
		return facts, nil
	}
	facts.Domain = domain
	if facts.Consumer = matcher.IsConsumer(facts.Host); facts.Consumer {
		return facts, nil
	}
	if facts.Suppressed, err = domainSuppressedTx(ctx, tx, domain); err != nil || facts.Suppressed {
		return facts, err
	}
	owners, err := exactCompanyByDomain(ctx, tx, []string{facts.Host, domain}, nil)
	if err != nil {
		return facts, err
	}
	facts.Owners, err = companyOwnerFacts(ctx, tx, owners)
	return facts, err
}

func companyOwnerFacts(ctx context.Context, tx pgx.Tx, companies []ids.CompanyID) ([]domainOwnerFacts, error) {
	if len(companies) == 0 {
		return nil, nil
	}
	rows, err := tx.Query(ctx, `SELECT id, owner_id FROM company WHERE id = ANY($1) ORDER BY id`, companies)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the owners of a domain's companies: %w", err)
	}
	owners, err := pgx.CollectRows(rows, pgx.RowToStructByPos[domainOwnerFacts])
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the owners of a domain's companies: %w", err)
	}
	return owners, nil
}

// employerDomainsVisible answers the domains a new company may take. It answers
// none for an address that is not a business one, or a domain held out of sight.
func employerDomainsVisible(ctx context.Context, tx pgx.Tx, f employerDomainFacts) ([]CompanyDomainInput, error) {
	if f.Domain == "" || f.Consumer || f.Suppressed {
		return nil, nil
	}
	for _, owner := range f.Owners {
		visible, err := auth.VisibleTo(ctx, tx, companyEntity, owner.CompanyID)
		if err != nil || !visible {
			return nil, err
		}
	}
	domains := []CompanyDomainInput{{Domain: f.Domain, IsPrimary: true}}
	if f.Host != f.Domain {
		domains = append(domains, CompanyDomainInput{Domain: f.Host})
	}
	return domains, nil
}

// promotionEmployerFacts are what the promotion's employer decision reads.
// Domain is nil when a linked company settles it without the address.
type promotionEmployerFacts struct {
	Format        string               `json:"format"`
	LinkedCompany *ids.UUID            `json:"linked_company"`
	CreateCompany bool                 `json:"create_company"`
	TypedName     string               `json:"typed_name"`
	Domain        *employerDomainFacts `json:"domain"`
}

func (s *Store) promotionEmployerFactsTx(ctx context.Context, tx pgx.Tx, lead crmcontracts.Lead, createCompany bool) (promotionEmployerFacts, error) {
	facts := promotionEmployerFacts{
		Format: promotionContextFormat, CreateCompany: createCompany, TypedName: strings.TrimSpace(deref(lead.CompanyName)),
	}
	if lead.CompanyId != nil {
		facts.LinkedCompany = new(ids.UUID(*lead.CompanyId))
	}
	if facts.LinkedCompany != nil || lead.Email == nil {
		return facts, nil
	}
	domain, err := s.employerDomainFactsTx(ctx, tx, string(*lead.Email))
	facts.Domain = &domain
	return facts, err
}

// digest binds the employer facts, the evidence and the contact a promotion
// lands on. The trigger is in it because a held meeting also proves a booking.
func (f promotionEmployerFacts) digest(trigger string, evidence *ids.ActivityID, target ids.ContactID) (string, error) {
	if f.Domain != nil {
		domain := *f.Domain
		domain.Owners = slices.Clone(domain.Owners)
		slices.SortFunc(domain.Owners, func(a, b domainOwnerFacts) int { return strings.Compare(a.CompanyID.String(), b.CompanyID.String()) })
		f.Domain = &domain
	}
	pinned := struct {
		promotionEmployerFacts
		Trigger  string    `json:"trigger"`
		Evidence *ids.UUID `json:"evidence"`
		Target   *ids.UUID `json:"merge_into"`
	}{promotionEmployerFacts: f, Trigger: trigger}
	if evidence != nil {
		pinned.Evidence = &evidence.UUID
	}
	if !target.IsZero() {
		pinned.Target = &target.UUID
	}
	raw, err := json.Marshal(pinned)
	if err != nil {
		return "", err
	}
	_, hash, err := diffhash.Canonical(raw)
	return hash, err
}

// PromotionContextDigestTx is the digest an approval of this promotion pins.
// A lead with no matching evidence digests none, and the promotion itself refuses it.
func (s *Store) PromotionContextDigestTx(ctx context.Context, tx pgx.Tx, id ids.LeadID, in PromoteLeadInput) (string, error) {
	if err := auth.Require(ctx, leadEntity, principal.ActionRead); err != nil {
		return "", err
	}
	lead, err := readLead(ctx, tx, id, storekit.IncludeArchived, nil)
	if err != nil {
		return "", err
	}
	evidence, err := promotionEvidence(ctx, tx, id, in)
	if _, missing := errors.AsType[*PromoteEvidenceError](err); missing {
		evidence, err = nil, nil
	}
	if err != nil {
		return "", err
	}
	target, err := s.leadPromotionTarget(ctx, tx, lead)
	if err != nil {
		return "", err
	}
	facts, err := s.promotionEmployerFactsTx(ctx, tx, lead, in.CreateCompany)
	if err != nil {
		return "", err
	}
	return facts.digest(in.Trigger, evidence, target)
}

// lockedPromotionEmployer is promotionEmployer for the write. It holds the rows
// the facts are read from, reads them again, and refuses an approval whose digest
// they no longer match.
//
// Lock order, all FOR SHARE: the lead, the companies holding its mail domain,
// their company_domain rows, the domain's admission row. Company writers also
// lock a company before its domains. A claim between the two reads is refused.
func (s *Store) lockedPromotionEmployer(ctx context.Context, tx pgx.Tx, lead crmcontracts.Lead, in PromoteLeadInput, candidate ids.ContactID) (EmployerChoice, string, error) {
	read, err := s.promotionEmployerFactsTx(ctx, tx, lead, in.CreateCompany)
	if err != nil {
		return EmployerChoice{}, "", err
	}
	if err := lockEmployerDomainRows(ctx, tx, read.Domain); err != nil {
		return EmployerChoice{}, "", err
	}
	employer, typed, facts, err := s.promotionEmployer(ctx, tx, lead, in.CreateCompany)
	if err != nil {
		return EmployerChoice{}, "", err
	}
	released, ok := auth.ReleasedRecordContext(ctx, leadEntity, ids.UUID(lead.Id))
	if !ok {
		return employer, typed, nil
	}
	current, err := facts.digest(in.Trigger, in.EvidenceActivityID, candidate)
	if err != nil {
		return EmployerChoice{}, "", err
	}
	if current != released {
		return EmployerChoice{}, "", auth.ReleasedContextMoved()
	}
	return employer, typed, nil
}

func lockEmployerDomainRows(ctx context.Context, tx pgx.Tx, domain *employerDomainFacts) error {
	if domain == nil || domain.Domain == "" {
		return nil
	}
	companies := make([]ids.UUID, 0, len(domain.Owners))
	for _, owner := range domain.Owners {
		companies = append(companies, owner.CompanyID)
	}
	for _, held := range []struct {
		statement string
		arg       any
	}{
		{`SELECT id FROM company WHERE id = ANY($1) ORDER BY id FOR SHARE`, companies},
		{`SELECT id FROM company_domain WHERE company_id = ANY($1) AND archived_at IS NULL ORDER BY id FOR SHARE`, companies},
		{`SELECT id FROM company_domain_disposition WHERE domain = $1 FOR SHARE`, domain.Domain},
	} {
		if _, err := tx.Exec(ctx, held.statement, held.arg); err != nil {
			return fmt.Errorf("contacts: holding the rows a promotion's employer reads: %w", err)
		}
	}
	return nil
}
