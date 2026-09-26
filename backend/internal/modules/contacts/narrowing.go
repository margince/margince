// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package contacts

// Why a contact is visible to its owner alone, recorded by the decision that
// narrowed it.
//
// A widening has to know which narrowing it is ending. A reply from the address
// ends "we wrote and nobody answered"; it must not end a confidentiality hold,
// an advisor verdict or a human's own "make private", and visibility alone cannot
// tell those apart. Re-deriving the reason when the reply arrives would read
// present state, so a hold lifted since would read as no hold at all.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/platform/database/storekit"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// NarrowingReason is the closed vocabulary of contact.narrowing_reason.
//
// Held by: TestTheNarrowingReasonVocabularyIsTheCatalogsCheck
// (backend/gates/narrowingreasonvocabulary_test.go), which fails when this set
// and the column's CHECK stop naming the same values.
type NarrowingReason string

const (
	// NarrowedOutboundNoAnswer: the owner wrote to the address and it has never
	// answered. The only reason a reply ends.
	NarrowedOutboundNoAnswer NarrowingReason = "outbound_no_answer"
	// NarrowedConfidentialityHold: the correspondence it came from is held.
	NarrowedConfidentialityHold NarrowingReason = "confidentiality_hold"
	// NarrowedAdvisor: a verdict judged the sender the owner's own advisor.
	NarrowedAdvisor NarrowingReason = "advisor"
	// NarrowedHumanDecided: a human made the contact private.
	NarrowedHumanDecided NarrowingReason = "human_decided"
)

// NarrowingReasons lists every value the column accepts.
func NarrowingReasons() []NarrowingReason {
	return []NarrowingReason{
		NarrowedOutboundNoAnswer, NarrowedConfidentialityHold, NarrowedAdvisor, NarrowedHumanDecided,
	}
}

// fieldNarrowingReason names the column in an audit image.
const fieldNarrowingReason = "narrowing_reason"

// recordNarrowingTx writes why an EXISTING owner-scoped contact stays its
// owner's, when a decision reaches a row an earlier ensure already minted.
//
// It only ever replaces nothing or outbound_no_answer. Every other reason is
// one a reply may not end, so overwriting one would let a later machine
// decision undo a hold, an advisor verdict or a human's choice.
func recordNarrowingTx(ctx context.Context, tx pgx.Tx, id ids.ContactID, reason NarrowingReason) error {
	var before *NarrowingReason
	err := tx.QueryRow(ctx, `
		SELECT narrowing_reason FROM contact
		 WHERE id = $1 AND visibility = $2 AND archived_at IS NULL
		 FOR UPDATE`, id, visibilityOwner).Scan(&before)
	if errors.Is(err, pgx.ErrNoRows) {
		// Workspace-visible or archived: this door never narrows, so there is
		// no owner-scoped row to explain.
		return nil
	}
	if err != nil {
		return fmt.Errorf("contacts: reading why a contact stays its owner's: %w", err)
	}
	if before != nil && (*before == reason || *before != NarrowedOutboundNoAnswer) {
		return nil
	}
	if _, err := tx.Exec(ctx,
		`UPDATE contact SET narrowing_reason = $2 WHERE id = $1`, id, reason); err != nil {
		return fmt.Errorf("contacts: recording why a contact stays its owner's: %w", err)
	}
	auditID, err := storekit.Audit(ctx, tx, "update", entityContact, id.UUID,
		map[string]any{fieldNarrowingReason: before},
		map[string]any{fieldNarrowingReason: reason})
	if err != nil {
		return fmt.Errorf("contacts: auditing why a contact stays its owner's: %w", err)
	}
	return storekit.EmitEvent(ctx, tx, auditID, id.UUID, crmcontracts.PublicEventContactUpdated{
		ChangedFields: map[string]any{fieldNarrowingReason: reason},
	})
}
