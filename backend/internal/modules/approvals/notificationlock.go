// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package approvals

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/systemtext"
)

// PendingAskedOfInTx reports whether the acting seat is the one member this
// pending request names, and may still decide it. Nobody else is told about it.
// It locks the row, ordering notification copies against decisions and erasure.
func (s *Service) PendingAskedOfInTx(ctx context.Context, tx pgx.Tx, id ids.ApprovalID) (bool, error) {
	return PendingAskedOfAtTx(ctx, tx, id, s.now())
}

// PendingAskOfInTx is PendingAskedOfInTx's answer with the request's summary
// parts, read under the same share lock. A notice about the request shows them
// as the card does. The parts are nil when the summary is a sentence alone.
func (s *Service) PendingAskOfInTx(ctx context.Context, tx pgx.Tx, id ids.ApprovalID) (bool, *crmcontracts.ApprovalSummaryParts, *systemtext.Message, error) {
	return PendingAskOfAtTx(ctx, tx, id, s.now())
}

// PendingAskOfAtTx is PendingAskOfInTx for a caller that holds only a
// transaction, as PendingAskedOfAtTx is.
func PendingAskOfAtTx(ctx context.Context, tx pgx.Tx, id ids.ApprovalID, now time.Time) (bool, *crmcontracts.ApprovalSummaryParts, *systemtext.Message, error) {
	request, parts, message, err := PendingNoticeTx(ctx, tx, id, func() time.Time { return now })
	return request != nil, parts, message, err
}

// PendingNoticeTx checks expiry after waiting for the request's eligibility lock.
func PendingNoticeTx(ctx context.Context, tx pgx.Tx, id ids.ApprovalID, clock func() time.Time) (*MovedRequest, *crmcontracts.ApprovalSummaryParts, *systemtext.Message, error) {
	decidable, err := pendingAskedOfWithClockTx(ctx, tx, id, clock)
	if err != nil || !decidable {
		return nil, nil, nil, err
	}
	request := MovedRequest{ID: id}
	var stored json.RawMessage
	if err := tx.QueryRow(ctx, `SELECT summary_parts, coalesce(summary,''), coalesce(target_entity_type,''), target_entity_id FROM approval WHERE id=@id`, pgx.StrictNamedArgs{"id": id}).Scan(&stored, &request.Summary, &request.TargetType, &request.TargetID); err != nil {
		return nil, nil, nil, fmt.Errorf("approvals: reading the request a notice shows: %w", err)
	}
	parts := readSummaryParts(stored)
	message := SummarySystemText(stored)
	if parts == nil || message == nil || !message.Subject.Matches(parts.Sentence) {
		message = nil
	}
	return &request, parts, message, nil
}

// PendingAskedOfAtTx is PendingAskedOfInTx for a caller that holds only a
// transaction, such as the deal writer handing a request to a new owner.
func PendingAskedOfAtTx(ctx context.Context, tx pgx.Tx, id ids.ApprovalID, now time.Time) (bool, error) {
	return pendingAskedOfWithClockTx(ctx, tx, id, func() time.Time { return now })
}

func pendingAskedOfWithClockTx(ctx context.Context, tx pgx.Tx, id ids.ApprovalID, clock func() time.Time) (bool, error) {
	if err := actingForAHuman(ctx); err != nil {
		return false, err
	}
	var locked ids.ApprovalID
	if err := tx.QueryRow(ctx, `SELECT id FROM approval WHERE id = @id `+lockOrder+` FOR SHARE`, pgx.StrictNamedArgs{"id": id}).Scan(&locked); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return false, nil
		}
		return false, err
	}
	return pendingDecidableInScopeAt(ctx, tx, id, ListInput{RequestedForReader: true}, clock())
}
