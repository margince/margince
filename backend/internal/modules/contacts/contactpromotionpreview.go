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

type ContactCreationPromotion struct {
	LeadID      ids.LeadID
	LeadVersion int64
	Patch       *storekit.Patch
	Phones      []ContactPhoneInput
	SourceCarry LeadPromotionCarry
}

func (s *Store) previewContactPromotions(ctx, caller context.Context, tx pgx.Tx, in CreateContactInput, matches []ids.LeadID) ([]ContactCreationPromotion, error) {
	current := crmcontracts.Contact{Title: in.Title}
	phones := map[string][]heldPhone{}
	for _, phone := range in.Phones {
		phones[phone.Phone] = append(phones[phone.Phone], heldPhone{phoneType: phone.PhoneType})
	}
	out := make([]ContactCreationPromotion, 0, len(matches))
	for _, id := range matches {
		lead, err := promotableLead(caller, tx, id, PromoteLeadInput{Trigger: string(TriggerHumanQualify)})
		if promotionRefused(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if err := auth.Require(ctx, "lead", principal.ActionUpdate); err != nil {
			return nil, err
		}
		if err := auth.EnsureWritable(ctx, tx, "lead", id.UUID); err != nil {
			return nil, err
		}
		if err := promotionFieldsVisible(ctx, "lead"); err != nil {
			return nil, err
		}
		if lead.Version == nil {
			return nil, apperrors.ErrPermissionDenied
		}
		var patch *storekit.Patch
		current, patch = promotionContactChange(lead, current)
		carried := promotionPhones(lead, phones)
		for _, phone := range carried {
			phones[phone.Phone] = append(phones[phone.Phone], heldPhone{phoneType: phone.PhoneType})
		}
		var source LeadPromotionCarry
		if err := s.previewPromotionCarry(ctx, tx, id, ids.ContactID{}, &source); err != nil {
			return nil, err
		}
		out = append(out, ContactCreationPromotion{LeadID: id, LeadVersion: int64(*lead.Version), Patch: patch, Phones: carried, SourceCarry: source})
	}
	return out, nil
}
