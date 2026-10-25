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

// ContactCreationPreview resolves creation defaults and identity before approval.
type ContactCreationPreview struct {
	Input           CreateContactInput
	CustomFields    map[string]any
	ReviewContactID *ids.ContactID
	Acquisition     Acquisition
	MatchingLeadIDs []ids.LeadID
	Promotions      []ContactCreationPromotion
	Social          []ContactSocialInput
}

// PreviewContactCreationForActor resolves authorized effects without applying the proposed change.
func (s *Store) PreviewContactCreationForActor(ctx context.Context, in CreateContactInput, creator principal.Principal) (ContactCreationPreview, error) {
	var out ContactCreationPreview
	if err := auth.Require(ctx, contactEntity, principal.ActionCreate); err != nil {
		return out, err
	}
	in.Emails, in.Phones = slices.Clone(in.Emails), slices.Clone(in.Phones)
	creatorCtx := principal.WithActor(ctx, creator)
	if _, err := s.readyContactCreate(creatorCtx, &in); err != nil {
		return out, err
	}
	active, err := s.activeColumns(ctx, contactEntity)
	if err != nil {
		return out, err
	}
	err = s.txSnapshot(ctx, func(tx pgx.Tx) error {
		owner, err := storekit.NewRecordOwnerWithDefault(ctx, tx, in.OwnerID, storekit.OwnerOrActor(creatorCtx))
		if err != nil {
			return err
		}
		in.OwnerID = owner
		match, err := admitAndDedupeContactCreate(ctx, tx, in)
		if err != nil {
			return err
		}
		if match.Decision == DecisionExactCollision && match.MatchedLane != lanePhone {
			return refusedContactCreate(ctx, tx, match, ContactSpec{Emails: in.Emails})
		}
		if err := storekit.RefuseUnknownSeat(ctx, tx, in.Author); err != nil {
			return err
		}
		out.ReviewContactID, err = contactCreationReviewTarget(ctx, tx, match)
		if err != nil {
			return err
		}
		out.Input = in
		if out.CustomFields, err = storekit.CreateFieldValues(active, in.CustomFields); err != nil {
			return err
		}
		if out.Social, err = contactSocialInputs(in.Social); err != nil {
			return err
		}
		out.Acquisition = normalizedAcquisition(acquisitionForCreate(in))
		allowed, err := mayPromoteHeldLeads(creatorCtx)
		if err != nil || !allowed {
			return err
		}
		out.MatchingLeadIDs, err = previewContactHeldLeads(ctx, tx, in.Emails)
		if err != nil {
			return err
		}
		out.Promotions, err = s.previewContactPromotions(ctx, creatorCtx, tx, in, out.MatchingLeadIDs)
		return err
	})
	if err != nil {
		return ContactCreationPreview{}, err
	}
	return out, nil
}

func admitAndDedupeContactCreate(ctx context.Context, tx pgx.Tx, in CreateContactInput) (ContactResolution, error) {
	if err := ensureContactEmailsUnclaimed(ctx, tx, in.Emails); err != nil {
		return ContactResolution{}, err
	}
	return manualDedupeContact(ctx, tx, in)
}

func contactCreationReviewTarget(ctx context.Context, tx pgx.Tx, match ContactResolution) (*ids.ContactID, error) {
	if match.Decision.queuesReview() {
		// Only a match the caller may see is named, and a hidden one changes
		// nothing: the preview reads as it would without that record.
		shown, err := firstVisibleReview(ctx, tx, match)
		if err != nil || shown == nil {
			return nil, err
		}
		return &shown.ContactID, nil
	}
	if match.Decision == DecisionExactCollision {
		if err := auth.Require(ctx, contactEntity, principal.ActionRead); err != nil {
			return nil, err
		}
		if err := auth.EnsureVisible(ctx, tx, contactEntity, match.ContactID.UUID); err != nil {
			return nil, err
		}
		return &match.ContactID, nil
	}
	return nil, nil //nolint:nilnil // A unique identity has no review target.
}
