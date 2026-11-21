// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/employment"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// CompanyAfterUpdate is the stored company current as an update leaves it. An
// empty name or a nil address keeps the stored one, and domain joins the rest.
func CompanyAfterUpdate(id ids.UUID, current []byte, displayName, legalName, domain string, address *crmcontracts.Address) (NewCompany, error) {
	var stored crmcontracts.Company
	if err := json.Unmarshal(current, &stored); err != nil {
		return NewCompany{}, fmt.Errorf("contacts: reading the stored company: %w", err)
	}
	held := NewCompany{
		ID: id, DisplayName: cmp.Or(displayName, stored.DisplayName), LegalName: cmp.Or(legalName, derefString(stored.LegalName)),
		Address: cmp.Or(address, stored.Address), Domains: []string{domain},
	}
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
// employer, as the contact ladder joins them, when the caller may see it.
func employerDomainsTx(ctx context.Context, tx pgx.Tx, contact ids.ContactID) ([]string, error) {
	rows, err := tx.Query(ctx, `
		SELECT r.company_id FROM relationship r
		 WHERE r.contact_id = $1 AND r.kind = 'employment' AND r.archived_at IS NULL
		   AND `+employment.CurrentPrimarySQL("r")+` ORDER BY r.company_id`, contact.UUID)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading a contact's employer: %w", err)
	}
	employers, err := pgx.CollectRows(rows, pgx.RowTo[ids.CompanyID])
	if err != nil {
		return nil, fmt.Errorf("contacts: reading a contact's employer: %w", err)
	}
	var domains []string
	for _, employer := range employers {
		held, err := visibleCompanyDomainsTx(ctx, tx, employer)
		if err != nil {
			return nil, err
		}
		domains = append(domains, held...)
	}
	return domains, nil
}

// visibleCompanyDomainsTx answers the live domains of a company the caller may
// see. It answers none for one they may not, whose domain would tell them it exists.
func visibleCompanyDomainsTx(ctx context.Context, tx pgx.Tx, company ids.CompanyID) ([]string, error) {
	visible, err := auth.VisibleTo(ctx, tx, entityCompany, company.UUID)
	if err != nil || !visible {
		return nil, err
	}
	rows, err := tx.Query(ctx, `
		SELECT domain FROM company_domain WHERE company_id = $1 AND archived_at IS NULL ORDER BY domain`, company.UUID)
	if err != nil {
		return nil, fmt.Errorf("contacts: reading a company's domains: %w", err)
	}
	domains, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, fmt.Errorf("contacts: reading a company's domains: %w", err)
	}
	return domains, nil
}
