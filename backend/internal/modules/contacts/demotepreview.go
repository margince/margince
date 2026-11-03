// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

import (
	"context"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// LeadDemotionPreview makes promotion unwinding reviewable under the writer’s row locks.
type LeadDemotionPreview struct {
	ContactID ids.ContactID
	Unwind    crmcontracts.DemoteLeadResponseUnwind
}

// PreviewLeadDemotion takes the writer's locks so both sides describe one current promotion.
func (s *Store) PreviewLeadDemotion(ctx context.Context, id ids.LeadID, opts ...WriteOption) (LeadDemotionPreview, error) {
	if err := auth.Require(ctx, leadEntity, principal.ActionRead); err != nil {
		return LeadDemotionPreview{}, err
	}
	if err := auth.Require(ctx, contactEntity, principal.ActionRead); err != nil {
		return LeadDemotionPreview{}, err
	}
	if err := auth.Require(ctx, leadEntity, principal.ActionUpdate); err != nil {
		return LeadDemotionPreview{}, err
	}
	if err := demotionFieldsWritable(ctx); err != nil {
		return LeadDemotionPreview{}, err
	}
	options := collectWriteOptions(opts)
	var out LeadDemotionPreview
	err := s.tx(ctx, func(tx pgx.Tx) error {
		contactID, err := probeBothSidesAndNameTheContact(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := lockContactThenLead(ctx, tx, contactID, storekit.LiveOnly, id, nil); err != nil {
			return err
		}
		lead, err := readLead(ctx, tx, id, storekit.IncludeArchived, nil)
		if err != nil {
			return err
		}
		if err := stillTheLeadTheCallerDescribed(lead, contactID, options); err != nil {
			return err
		}
		outcome, err := promotedOutcome(ctx, tx, id)
		if err != nil {
			return err
		}
		if err := refuseIfColleagueWorkedOnCreated(ctx, tx, contactID, outcome, options, false); err != nil {
			return err
		}
		unwind, err := demotionUnwind(ctx, tx, id, contactID, outcome)
		if err != nil {
			return err
		}
		out = LeadDemotionPreview{ContactID: contactID, Unwind: unwind}
		return nil
	})
	if err != nil {
		return LeadDemotionPreview{}, err
	}
	return out, nil
}

func demotionUnwind(ctx context.Context, tx pgx.Tx, leadID ids.LeadID, contactID ids.ContactID, outcome promotionOutcome) (crmcontracts.DemoteLeadResponseUnwind, error) {
	if err := ensureNoLiveDeal(ctx, tx, contactID); err != nil {
		return "", err
	}
	if outcome != outcomeCreated {
		return "", &ContactStaysError{Merged: true}
	}
	shared, err := isSharedByOthers(ctx, tx, leadID, contactID)
	if err != nil {
		return "", err
	}
	if shared {
		return "", &ContactStaysError{}
	}
	return crmcontracts.DemoteLeadResponseUnwindDemoteUnwindReversed, nil
}

func demotionFieldsWritable(ctx context.Context) error {
	actor, ok := principal.Actor(ctx)
	if !ok {
		return apperrors.ErrPermissionDenied
	}
	for _, field := range auth.MaskedFields(actor, leadEntity, true) {
		if field == fieldKeyPromotedContact || field == leadStatusColumn {
			return apperrors.ErrPermissionDenied
		}
	}
	return nil
}
