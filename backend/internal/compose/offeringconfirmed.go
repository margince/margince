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

// offeringConfirmed reads whether the offering a company is judged against is
// confirmed (DOSS-AC-13), plus a digest of it for the cache key.
//
// Both come from one read of the context the growth-fit task sends, so they
// cannot describe two states. Only the digest travels, never the text (DOSS-AC-6).
// A missing offering reads as not confirmed, so onboarding still gets capped bands.
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
