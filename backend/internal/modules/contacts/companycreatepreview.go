// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// CompanyCreationPreview resolves creation defaults and identity before approval.
type CompanyCreationPreview struct {
	Input           CreateCompanyInput
	CustomFields    map[string]any
	ReviewCompanyID *ids.CompanyID
	Geocode         bool
	AddressLookup   bool
}

// PreviewCompanyCreationForActor resolves authorized effects without applying the proposed change.
func (s *Store) PreviewCompanyCreationForActor(ctx context.Context, in CreateCompanyInput, creator principal.Principal) (CompanyCreationPreview, error) {
	var out CompanyCreationPreview
	if err := auth.Require(ctx, companyEntity, principal.ActionCreate); err != nil {
		return out, err
	}
	// Domain normalization mutates its slice; a preview does not edit stored arguments.
	in.Domains = slices.Clone(in.Domains)
	creatorCtx := principal.WithActor(ctx, creator)
	if _, err := s.readyCompanyCreate(creatorCtx, &in); err != nil {
		return out, err
	}
	active, err := s.activeColumns(ctx, companyEntity)
	if err != nil {
		return out, err
	}
	err = s.txSnapshot(ctx, func(tx pgx.Tx) error {
		owner, err := storekit.NewRecordOwnerWithDefault(ctx, tx, in.OwnerID, storekit.OwnerOrActor(creatorCtx))
		if err != nil {
			return err
		}
		in.OwnerID = owner
		match, err := admitAndDedupeCompanyCreate(ctx, tx, in)
		if err != nil {
			return err
		}
		if match.Decision == DecisionExactCollision {
			return refusedCompanyCreate(ctx, tx, match, CompanySpec{Domains: in.Domains})
		}
		if err := storekit.RefuseUnknownSeat(ctx, tx, in.Author); err != nil {
			return err
		}
		if match.Decision == DecisionFuzzyReview {
			if err := auth.Require(ctx, companyEntity, principal.ActionRead); err != nil {
				return err
			}
			if err := auth.EnsureVisible(ctx, tx, companyEntity, match.CompanyID.UUID); err != nil {
				return err
			}
			out.ReviewCompanyID = &match.CompanyID
		}
		in.Domains = electPrimary(in.Domains, "")
		out.Input = in
		if out.CustomFields, err = storekit.CreateFieldValues(active, in.CustomFields); err != nil {
			return err
		}
		out.AddressLookup = namesAPlace(in.Address)
		out.Geocode = s.geocodeEnqueue != nil && out.AddressLookup
		return nil
	})
	if err != nil {
		return CompanyCreationPreview{}, err
	}
	return out, nil
}

func ensureCompanyLinkedInUnclaimed(ctx context.Context, tx pgx.Tx, url *string) error {
	if url == nil {
		return nil
	}
	var claimed bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM company WHERE archived_at IS NULL AND lower(linkedin_url)=lower(@url))`, pgx.NamedArgs{"url": *url}).Scan(&claimed); err != nil {
		return err
	}
	if claimed {
		return &DuplicateCompanyLinkedInError{URL: *url}
	}
	return nil
}
