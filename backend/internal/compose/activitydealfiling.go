// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/modules/deals"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// ActivityDealFiling resolves a logged activity's records to their deals.
type ActivityDealFiling struct{ deals *deals.Store }

// NewActivityDealFiling builds the resolver over the deals store.
func NewActivityDealFiling(db *database.DB) ActivityDealFiling {
	return ActivityDealFiling{deals: deals.NewStore(db, DealsInstallation())}
}

// Resolve answers the one company the records stand on and its open deals.
func (f ActivityDealFiling) Resolve(ctx context.Context, tx pgx.Tx, records []activities.ActivityLinkInput) (crmcontracts.ActivityDealFilingOptions, error) {
	out := crmcontracts.ActivityDealFilingOptions{}
	companies, err := f.CompaniesOfRecords(ctx, tx, records)
	if err != nil || len(companies) != 1 {
		return out, err
	}
	company := companies[0]
	out.CompanyId = new(crmcontracts.Id(company))
	if !auth.ReadGranted(ctx, "deal") {
		return out, nil
	}
	candidates, err := f.deals.OpenDealsOfCompany(ctx, tx, company)
	if err != nil {
		return out, err
	}
	// Manual logs ask on several deals, even when capture could use a thread or stakeholder seat.
	if len(candidates) > 1 {
		out.ChoiceRequired = true
		return out, nil
	}
	if len(candidates) == 0 {
		return out, nil
	}
	// The link writer locks the deal before checking attach permission.
	out.SuggestedDealId = new(crmcontracts.Id(candidates[0]))
	return out, nil
}

// IsOpenDealOfRecords answers whether deal is an open deal of a company the
// records stand on.
func (f ActivityDealFiling) IsOpenDealOfRecords(ctx context.Context, tx pgx.Tx, records []activities.ActivityLinkInput, deal ids.UUID) (bool, error) {
	companies, err := f.CompaniesOfRecords(ctx, tx, records)
	if err != nil || len(companies) == 0 {
		return false, err
	}
	return f.deals.IsOpenDealOfCompanies(ctx, tx, deal, companies)
}

// CompaniesOfRecords answers the companies the contacts and companies stand on,
// after the caller's read gate on each record.
func (f ActivityDealFiling) CompaniesOfRecords(ctx context.Context, tx pgx.Tx, records []activities.ActivityLinkInput) ([]ids.UUID, error) {
	companies := map[ids.UUID]bool{}
	var contactIDs []ids.UUID
	for _, record := range records {
		if record.EntityType != string(recordTypeContact) && record.EntityType != string(recordTypeCompany) {
			continue
		}
		var admission error
		if record.EntityType == string(recordTypeCompany) {
			admission = auth.Require(ctx, "company", principal.ActionRead)
		} else {
			admission = auth.Require(ctx, "contact", principal.ActionRead)
		}
		if admission != nil {
			return nil, admission
		}
		if err := auth.EnsureVisibleLive(ctx, tx, record.EntityType, record.EntityID); err != nil {
			return nil, err
		}
		if record.EntityType == string(recordTypeCompany) {
			companies[record.EntityID] = true
		} else {
			contactIDs = append(contactIDs, record.EntityID)
		}
	}
	if len(contactIDs) > 0 {
		employers, err := contacts.CurrentEmployersTx(ctx, tx, contactIDs)
		if err != nil && !errors.Is(err, apperrors.ErrPermissionDenied) {
			return nil, err
		}
		for _, employer := range employers {
			companies[ids.UUID(employer.CompanyId)] = true
		}
	}
	out := make([]ids.UUID, 0, len(companies))
	for company := range companies {
		out = append(out, company)
	}
	return out, nil
}
