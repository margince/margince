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

// offeringConfirmed reports whether the offering a company is judged against
// is described well enough for a growth fit to be measured against it
// (DOSS-AC-13), and digests what it says so a cached fit is invalidated when
// that changes.
//
// "Confirmed" and the fingerprint both come from ONE read of the company
// context the growth-fit task sends, so they cannot describe two different
// states of the offering. Confirmed is the bar onboarding uses to decide the
// installation has finished describing itself: a name, an offer summary and an
// ideal-customer profile. The main offering is the anchor company's profile; a
// legal entity named by entityID carries its own.
//
// The content never leaves as text — a fit derived from what WE sell is an
// assessment about THEM and must still cite their records (DOSS-AC-6). Only the
// digest travels, onto a cache key nothing renders.
//
// An offering that does not exist yet (an installation that has never saved its
// company, or an entity archived since it was chosen) reads as NOT confirmed
// rather than as an error: a workspace mid-onboarding should still get capped
// bands with the reason spelled out, not a broken panel.
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
