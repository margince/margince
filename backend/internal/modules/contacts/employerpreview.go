// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// Employer effects keep import planning aligned with store previews.
const (
	ContactEmployerCreateEffect  = "create_contact_employer"
	ContactEmployerResolveEffect = "resolve_contact_employer"
)

// ContactEmployerPreview discloses the resolver's current decision without writing it.
type ContactEmployerPreview struct {
	CompanyID   *ids.CompanyID
	CompanyName string
	Domains     []CompanyDomainInput
	Effect      string
}

// PreviewContactEmployer uses the write path's match and authority checks for import review.
func (s *Store) PreviewContactEmployer(ctx context.Context, in CreateContactInput) (ContactEmployerPreview, error) {
	if err := auth.Require(ctx, contactEntity, principal.ActionCreate); err != nil {
		return ContactEmployerPreview{}, err
	}
	var out ContactEmployerPreview
	err := s.txSnapshot(ctx, func(tx pgx.Tx) error {
		var err error
		out, err = s.previewContactEmployerTx(ctx, tx, in.Employer, in.Emails)
		return err
	})
	return out, err
}

func (s *Store) previewContactEmployerTx(ctx context.Context, tx pgx.Tx, choice EmployerChoice, emails []ContactEmailInput) (ContactEmployerPreview, error) {
	if choice.CompanyID == nil && len(choice.Domains) == 0 {
		var email string
		for _, candidate := range emails {
			if candidate.EmailType == emailTypeWork && (email == "" || candidate.IsPrimary) {
				email = candidate.Email
			}
		}
		var err error
		choice.Domains, err = s.employerDomainsForAddressTx(ctx, tx, email)
		if err != nil {
			return ContactEmployerPreview{}, err
		}
	}
	if choice.named() {
		out, err := s.previewNamedEmployerTx(ctx, tx, choice)
		out.CompanyName, out.Domains = deref(choice.CompanyName), choice.Domains
		return out, err
	}
	if len(choice.Domains) == 0 {
		return ContactEmployerPreview{}, nil
	}
	return s.previewDomainEmployerTx(ctx, tx, choice.Domains[len(choice.Domains)-1].Domain)
}

func (s *Store) previewDomainEmployerTx(ctx context.Context, tx pgx.Tx, domain string) (ContactEmployerPreview, error) {
	if err := auth.Require(ctx, companyEntity, principal.ActionCreate); err != nil {
		return ContactEmployerPreview{}, err
	}
	match, _, ok, err := employerMatchTx(ctx, tx, domain)
	if err != nil || !ok {
		return ContactEmployerPreview{}, err
	}
	if match.Decision == DecisionExactCollision {
		visible, err := auth.VisibleTo(ctx, tx, companyEntity, match.CompanyID.UUID)
		if err != nil || !visible {
			return ContactEmployerPreview{}, err
		}
		return ContactEmployerPreview{CompanyID: &match.CompanyID, Effect: ContactEmployerResolveEffect}, nil
	}
	return ContactEmployerPreview{Effect: ContactEmployerResolveEffect}, nil
}

func (s *Store) previewNamedEmployerTx(ctx context.Context, tx pgx.Tx, choice EmployerChoice) (ContactEmployerPreview, error) {
	if err := auth.Require(ctx, "relationship", principal.ActionCreate); err != nil {
		return ContactEmployerPreview{}, err
	}
	if err := auth.Require(ctx, contactEntity, principal.ActionUpdate); err != nil {
		return ContactEmployerPreview{}, err
	}
	id, err := matchNamedEmployer(ctx, tx, choice)
	if err != nil {
		return ContactEmployerPreview{}, err
	}
	if id != nil {
		return ContactEmployerPreview{CompanyID: id, Effect: "reuse_contact_employer"}, nil
	}
	if err := auth.Require(ctx, companyEntity, principal.ActionCreate); err != nil {
		return ContactEmployerPreview{}, err
	}
	return ContactEmployerPreview{Effect: ContactEmployerCreateEffect}, nil
}
