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

type ContactCreationPreview struct {
	Input           CreateContactInput
	CustomFields    map[string]any
	ReviewContactID *ids.ContactID
	Acquisition     Acquisition
	MatchingLeadIDs []ids.LeadID
	Promotions      []ContactCreationPromotion
	Social          []ContactSocialInput
}

func (s *Store) PreviewContactCreationForActor(ctx context.Context, in CreateContactInput, creator principal.Principal) (ContactCreationPreview, error) {
	var out ContactCreationPreview
	if err := auth.Require(ctx, "contact", principal.ActionCreate); err != nil {
		return out, err
	}
	in.Emails, in.Phones = slices.Clone(in.Emails), slices.Clone(in.Phones)
	creatorCtx := principal.WithActor(ctx, creator)
	if _, err := s.readyContactCreate(creatorCtx, &in); err != nil {
		return out, err
	}
	active, err := s.activeColumns(ctx, "contact")
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
		switch match.Decision {
		case DecisionFuzzyReview, DecisionNameCollisionReview, DecisionExactCollision:
			if err := auth.Require(ctx, "contact", principal.ActionRead); err != nil {
				return err
			}
			if err := auth.EnsureVisible(ctx, tx, "contact", match.ContactID.UUID); err != nil {
				return err
			}
			out.ReviewContactID = &match.ContactID
		}
		out.Input, out.CustomFields = in, storekit.CreateFieldValues(active, in.CustomFields)
		out.Social = contactSocialInputs(in.Social)
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
