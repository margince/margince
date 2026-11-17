// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"slices"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

func instructionScopeStillFits(ctx context.Context, tx pgx.Tx, recorded []RefusedRecipient, current commsauthz.DecisionSet) error {
	for _, now := range refusedRecipientsOf(current) {
		found := false
		for _, before := range recorded {
			if before.Address != now.Address || before.SubjectKind != now.SubjectKind || before.SubjectID != now.SubjectID {
				continue
			}
			if before.SendingEntity == nil && now.SendingEntity != nil {
				entity, err := qualifyingWriterEntity(ctx, tx, new(ids.UUID))
				if err != nil {
					return err
				}
				before.SendingEntity = &entity
			}
			if sameInstructionScope(before, now) {
				found = true
				break
			}
		}
		if !found {
			return &InstructionStaleError{Why: "the recipient, sending company or purpose has changed; check this message again"}
		}
	}
	return nil
}

func sameInstructionScope(before, now RefusedRecipient) bool {
	return before.claim() == now.claim() && sameOptionalID(before.SendingEntity, now.SendingEntity) &&
		sameOptionalID(before.PurposeID, now.PurposeID) && sameOptionalID(before.DealID, now.DealID)
}

func sameOptionalID(a, b *ids.UUID) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

// everyRefusalWasShown reports whether the human's decision saw each refusal
// that stands now: the same recipient refused for the same reason. A reason
// recorded after the decision, a new stop included, needs a decision of its own.
// Staging and dispatch both ask it.
func everyRefusalWasShown(shown []RefusedRecipient, current commsauthz.DecisionSet) bool {
	for _, now := range refusedRecipientsOf(current) {
		if !slices.ContainsFunc(shown, func(before RefusedRecipient) bool {
			return before.Address == now.Address && before.SubjectKind == now.SubjectKind &&
				before.SubjectID == now.SubjectID && before.ReasonCode == now.ReasonCode
		}) {
			return false
		}
	}
	return true
}
