// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"fmt"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

func instructionScopeStillFits(ctx context.Context, tx pgx.Tx, recorded []RefusedRecipient, current commsauthz.DecisionSet) error {
	for _, now := range refusedRecipientsOf(current) {
		found := false
		for _, before := range recorded {
			if !sameRecipient(before, now) {
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

// sameRecipient reports whether two refusals are about one recipient.
func sameRecipient(before, now RefusedRecipient) bool {
	return before.Address == now.Address && before.SubjectKind == now.SubjectKind && before.SubjectID == now.SubjectID
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
			return sameRecipient(before, now) && before.ReasonCode == now.ReasonCode
		}) {
			return false
		}
	}
	return true
}

// instructionStillAnswers reports whether a decision may stand over the
// refusals now. Its review must have shown each of them, and no stop or
// withdrawal may have been recorded for a refused subject since the decision:
// a new stop can hide behind a reason the review already named.
func instructionStillAnswers(ctx context.Context, tx pgx.Tx, shown []RefusedRecipient, directedAt time.Time, current commsauthz.DecisionSet) (bool, error) {
	refused := refusedRecipientsOf(current)
	if !everyRefusalWasShown(shown, current) {
		return false, nil
	}
	var contacts, leads []string
	addresses := make([]string, 0, len(refused))
	for _, r := range refused {
		addresses = append(addresses, r.Address)
		switch r.SubjectKind {
		case entityContact:
			contacts = append(contacts, r.SubjectID)
		case entityLead:
			leads = append(leads, r.SubjectID)
		}
	}
	var stopped bool
	err := tx.QueryRow(ctx, `
		SELECT EXISTS (
		         SELECT 1 FROM communication_suppression s
		          WHERE s.recorded_at > $1
		            AND (s.contact_id = ANY($2::uuid[]) OR s.lead_id = ANY($3::uuid[])
		                 OR lower(s.address) = ANY($4::text[])))
		    OR EXISTS (
		         SELECT 1 FROM consent_event e
		          WHERE e.new_state = 'withdrawn' AND e.captured_at > $1
		            AND (e.contact_id = ANY($2::uuid[]) OR e.lead_id = ANY($3::uuid[])))`,
		directedAt, contacts, leads, addresses).Scan(&stopped)
	if err != nil {
		return false, fmt.Errorf("consent: reading the stops recorded since the decision: %w", err)
	}
	return !stopped, nil
}
