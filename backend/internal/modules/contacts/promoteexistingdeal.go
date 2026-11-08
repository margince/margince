// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/platform/httperr"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
	"github.com/margince/margince/backend/internal/shared/ports/datasource"
)

// PromotionPreviewOptions carries the employer choice and continuation of eligible deals.
type PromotionPreviewOptions struct {
	CreateCompany bool
	DealsCursor   *string
}

// LeadDealSelector delegates deal eligibility and locks to its owning store.
type LeadDealSelector interface {
	QualificationDeals(context.Context, pgx.Tx, ids.CompanyID, *string) ([]crmcontracts.Deal, storekit.Page, error)
	HoldQualificationDeal(context.Context, pgx.Tx, ids.CompanyID, ids.UUID) error
}

func validatePromotionDeal(in PromoteLeadInput) error {
	return datasource.ValidatePromotionDeals(datasource.PromotionDealSelection{New: in.Deal != nil, Existing: in.ExistingDealID != nil})
}

func (s *Store) qualifiedDeal(ctx context.Context, tx pgx.Tx, leadID ids.LeadID, lead crmcontracts.Lead, contactID ids.ContactID, in PromoteLeadInput) (*ids.UUID, error) {
	if in.ExistingDealID == nil {
		if in.Deal == nil {
			return nil, nil //nolint:nilnil // Qualification without a deal needs no employer lookup.
		}
		company, err := currentEmployerID(ctx, tx, contactID)
		if err != nil {
			return nil, err
		}
		return s.openQualifiedDeal(ctx, tx, leadID, lead, contactID, company, in.Deal)
	}
	if err := s.seatQualifiedContact(ctx, tx, contactID, *in.ExistingDealID, promotionSource(leadID)); err != nil {
		return nil, err
	}
	return in.ExistingDealID, nil
}

func (s *Store) holdPromotionDeal(ctx context.Context, tx pgx.Tx, lead crmcontracts.Lead, employer EmployerChoice, candidate ids.ContactID, deal *ids.UUID) error {
	if deal == nil {
		return nil
	}
	company, err := s.promotionDealEmployer(ctx, tx, lead, employer, candidate)
	if err != nil {
		return err
	}
	if company == nil {
		return httperr.Validation("existing_deal_id", "company_required", "Resolve the contact's employer before linking its deal.")
	}
	return s.holdQualificationDeal(ctx, tx, *company, *deal)
}

func (s *Store) holdQualificationDeal(ctx context.Context, tx pgx.Tx, company ids.CompanyID, deal ids.UUID) error {
	if err := requireRelationshipCreate(ctx, anchorDeal); err != nil {
		return err
	}
	selector, ok := s.dealOpener.(LeadDealSelector)
	if !ok {
		return &DealOpenerNotWiredError{}
	}
	return selector.HoldQualificationDeal(ctx, tx, company, deal)
}

func (s *Store) promotionDealCompany(ctx context.Context, tx pgx.Tx, lead crmcontracts.Lead, createsCompany bool, contact ids.ContactID) (*ids.CompanyID, error) {
	employer, _, _, err := s.promotionEmployer(ctx, tx, lead, createsCompany)
	if err != nil {
		return nil, err
	}
	return s.promotionDealEmployer(ctx, tx, lead, employer, contact)
}

func (s *Store) promotionDealEmployer(ctx context.Context, tx pgx.Tx, lead crmcontracts.Lead, employer EmployerChoice, contact ids.ContactID) (*ids.CompanyID, error) {
	if !employer.named() && !contact.IsZero() {
		held, err := hasEmploymentHistory(ctx, tx, contact)
		if err != nil {
			return nil, err
		}
		if held {
			return currentEmployerID(ctx, tx, contact)
		}
	}
	spec := promotionContactSpec(lead, employer, leadIdentityName(lead), "")
	preview, err := s.previewContactEmployerTx(ctx, tx, employer, spec.Emails)
	if err != nil {
		return nil, err
	}
	if preview.CompanyID != nil || contact.IsZero() {
		return preview.CompanyID, nil
	}
	return currentEmployerID(ctx, tx, contact)
}

func (s *Store) promotionDealsPreview(ctx context.Context, tx pgx.Tx, lead crmcontracts.Lead, options PromotionPreviewOptions, match ContactResolution, out *crmcontracts.PromoteLeadPreview) error {
	selector, ok := s.dealOpener.(LeadDealSelector)
	if !ok {
		return nil
	}
	if actor, ok := principal.Actor(ctx); ok && actor.SeatType == principal.SeatRead {
		return nil
	}
	if err := qualificationDealGrant(ctx); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) {
			return nil
		}
		return err
	}
	company, err := s.promotionCandidateCompany(ctx, tx, lead, options, match)
	if err != nil || company == nil {
		return err
	}
	found, page, err := selector.QualificationDeals(ctx, tx, *company, options.DealsCursor)
	if err != nil {
		return err
	}
	id := crmcontracts.Id(company.UUID)
	out.CompanyId, out.ExistingDeals = &id, &found
	if page.HasMore {
		out.ExistingDealsCursor = &page.NextCursor
	}
	return nil
}

func (s *Store) seatQualifiedContact(ctx context.Context, tx pgx.Tx, contact ids.ContactID, deal ids.UUID, source string) error {
	if err := requireRelationshipCreate(ctx, anchorDeal); err != nil {
		return err
	}
	if err := auth.EnsureWritableLive(ctx, tx, "deal", deal); err != nil {
		return err
	}
	args := []any{contact, deal}
	var exists bool
	err := tx.QueryRow(ctx, storekit.SQLf(`SELECT EXISTS (SELECT 1 FROM relationship WHERE kind='deal_stakeholder' AND contact_id=$%d AND deal_id=$%d AND archived_at IS NULL)`, len(args)-1, len(args)), args...).Scan(&exists)
	if err != nil || exists {
		return err
	}
	seated := ids.From[ids.DealKind](deal)
	_, err = s.CreateRelationshipTx(ctx, tx, CreateRelationshipInput{Kind: dealStakeholderKind, ContactID: &contact, DealID: &seated, Source: source})
	return err
}

func qualificationDealGrant(ctx context.Context) error {
	if err := auth.Require(ctx, "deal", principal.ActionRead); err != nil {
		return err
	}
	if err := auth.Require(ctx, "deal", principal.ActionUpdate); err != nil {
		return err
	}
	return auth.Require(ctx, "relationship", principal.ActionCreate)
}

//nolint:nilnil // An unavailable optional company offers no deals without preventing contact qualification.
func (s *Store) promotionCandidateCompany(ctx context.Context, tx pgx.Tx, lead crmcontracts.Lead, options PromotionPreviewOptions, match ContactResolution) (*ids.CompanyID, error) {
	var candidate ids.ContactID
	if match.Decision == DecisionExactCollision {
		candidate = match.ContactID
		if err := promotionCandidateVisible(ctx, tx, candidate); err != nil {
			if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
				return nil, nil
			}
			return nil, err
		}
	}
	company, err := s.promotionDealCompany(ctx, tx, lead, options.CreateCompany, candidate)
	if err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	if company == nil {
		return nil, nil
	}
	if err := auth.EnsureReadable(ctx, tx, companyEntity, company.UUID); err != nil {
		if errors.Is(err, apperrors.ErrPermissionDenied) || errors.Is(err, apperrors.ErrNotFound) {
			return nil, nil
		}
		return nil, err
	}
	return company, nil
}
