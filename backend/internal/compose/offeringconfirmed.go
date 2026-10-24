// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The cross-module edge from the growth fit to this workspace's own company
// record, injected here because that is where every such edge is injected.

import (
	"context"
	"errors"

	"github.com/margince/margince/backend/internal/compose/companydossier"
	"github.com/margince/margince/backend/internal/modules/ai"
	"github.com/margince/margince/backend/internal/modules/contacts"
	"github.com/margince/margince/backend/internal/shared/apperrors"
)

// offeringConfirmed reads the offering a company is judged against: whether it
// is confirmed (DOSS-AC-13) and a digest of what it says, so a cached fit is
// invalidated when that changes.
//
// Confirmed and the digest come from one read of the context the growth-fit task
// sends, so they cannot describe two states. The content never leaves as text:
// a fit derived from what we sell must still cite their records (DOSS-AC-6).
//
// An offering that does not exist yet reads as not confirmed instead of as an
// error, so a workspace mid-onboarding still gets capped bands with the reason.
func offeringConfirmed(store *contacts.Store) companydossier.SelfOffering {
	return func(ctx context.Context, entityID string) (companydossier.Offering, error) {
		return readOffering(ctx, store, entityID)
	}
}

type companyContextStore interface {
	GetCompanyContext(context.Context, []contacts.CompanyContextScope, contacts.OfferingSelector) (contacts.CompanyContext, error)
}

func readOffering(ctx context.Context, store companyContextStore, entityID string) (companydossier.Offering, error) {
	selector, err := contacts.ParseOfferingSelector(entityID)
	if err != nil {
		return companydossier.Offering{}, err
	}
	scopes, err := companyContextScopesFor(ai.TaskGrowthFit)
	if err != nil {
		return companydossier.Offering{}, err
	}
	assembled, err := store.GetCompanyContext(ctx, scopes, selector)
	if errors.Is(err, apperrors.ErrNotFound) {
		return companydossier.Offering{}, nil
	}
	if err != nil {
		return companydossier.Offering{}, err
	}
	offering := companydossier.Offering{
		Confirmed: assembled.Confirmed, Fingerprint: assembled.Fingerprint,
		Name: assembled.OfferingName,
	}
	if !assembled.OfferingID.IsZero() {
		offering.ID = assembled.OfferingID.String()
		offering.IsDefault = selector.EntityID.IsZero()
	}
	return offering, nil
}
