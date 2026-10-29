// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/freemail"
	"github.com/margince/margince/backend/internal/platform/owndomain"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// linkEmployerSource is the provenance of an employer a reader attached by hand.
const linkEmployerSource = "manual"

// LinkEmployerInput is the employer to attach to a contact that already exists.
type LinkEmployerInput struct {
	CompanyID   *ids.CompanyID
	CompanyName *string
	Title       *string
}

// LinkEmployerResult is the contact as it now reads, what was written, and the
// domain a created company was given.
type LinkEmployerResult struct {
	Contact crmcontracts.Contact
	EmployerAttached
	CompanyDomain *string
}

// LinkContactEmployer attaches an employer to a contact the caller may change,
// creating the company when it is named rather than picked. The company, its
// domain and the edge land in one transaction or not at all.
func (s *Store) LinkContactEmployer(ctx context.Context, contactID ids.ContactID, in LinkEmployerInput) (LinkEmployerResult, error) {
	choice, err := employerChoiceFromRequest(in.CompanyID, in.CompanyName, linkEmployerSource)
	if err != nil {
		return LinkEmployerResult{}, err
	}
	if !choice.named() {
		return LinkEmployerResult{}, &RequiredFieldError{Field: missingEmployerField(in)}
	}
	choice.Role = in.Title
	if err := auth.Require(ctx, contactEntity, principal.ActionUpdate); err != nil {
		return LinkEmployerResult{}, err
	}
	active, err := s.activeColumns(ctx, contactEntity)
	if err != nil {
		return LinkEmployerResult{}, err
	}
	var out LinkEmployerResult
	err = s.txRetryingLockCycles(ctx, func(tx pgx.Tx) error {
		out = LinkEmployerResult{}
		if err := auth.EnsureWritableLive(ctx, tx, contactEntity, contactID.UUID); err != nil {
			return err
		}
		if choice.CompanyID == nil {
			domains, err := s.newEmployerDomainsTx(ctx, tx, contactID)
			if err != nil {
				return err
			}
			choice.Domains = domains
			if len(domains) > 0 {
				out.CompanyDomain = &domains[0].Domain
			}
		}
		attached, err := s.attachEmployerTx(ctx, tx, contactID, choice)
		if err != nil {
			return err
		}
		out.EmployerAttached = attached
		out.Contact, err = readContact(ctx, tx, contactID, storekit.LiveOnly, active)
		return err
	})
	return out, err
}

// missingEmployerField names the field a caller who named no employer should
// fill: the blank name they sent, else the id.
func missingEmployerField(in LinkEmployerInput) string {
	if in.CompanyName != nil {
		return fieldCompanyName
	}
	return endCompanyColumn
}

// newEmployerDomainsTx is the domain a company created for this contact may
// carry: their work address's registrable domain. Consumer mail, a domain refused
// a company and one another company holds give none. A holder the caller can
// read refuses the write with DuplicateDomainError, so the reader links it instead.
func (s *Store) newEmployerDomainsTx(ctx context.Context, tx pgx.Tx, contactID ids.ContactID) ([]CompanyDomainInput, error) {
	var email string
	err := tx.QueryRow(ctx, `
		SELECT email FROM contact_email
		 WHERE contact_id = $1 AND email_type = $2 AND archived_at IS NULL`+
		ReachableEmailOrder+` LIMIT 1`, contactID, emailTypeWork).Scan(&email)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("contacts: reading the work address of a contact: %w", err)
	}
	matcher, err := s.consumerMailMatcher(ctx, tx)
	if err != nil {
		return nil, err
	}
	host := owndomain.OfAddress(email)
	domain, ok := freemail.Hostname(host)
	if !ok || matcher.IsConsumer(host) {
		return nil, nil
	}
	suppressed, err := domainSuppressedTx(ctx, tx, domain)
	if err != nil || suppressed {
		return nil, err
	}
	err = claimedDomainOwner(ctx, tx, ids.CompanyID{}, domain)
	if taken, ok := errors.AsType[*DuplicateDomainError](err); ok && taken.ExistingID.IsZero() {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return []CompanyDomainInput{{Domain: domain, IsPrimary: true}}, nil
}
