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
func (s *Service) PendingAskOfInTx(ctx context.Context, tx pgx.Tx, id ids.ApprovalID) (bool, *crmcontracts.ApprovalSummaryParts, error) {
	return PendingAskOfAtTx(ctx, tx, id, s.now())
}

// PendingAskOfAtTx is PendingAskOfInTx for a caller that holds only a
// transaction, as PendingAskedOfAtTx is.
func PendingAskOfAtTx(ctx context.Context, tx pgx.Tx, id ids.ApprovalID, now time.Time) (bool, *crmcontracts.ApprovalSummaryParts, error) {
	decidable, err := PendingAskedOfAtTx(ctx, tx, id, now)
	if err != nil || !decidable {
		return false, nil, err
	}
	var stored json.RawMessage
	if err := tx.QueryRow(ctx, `SELECT summary_parts FROM approval WHERE id = @id`, pgx.StrictNamedArgs{"id": id}).Scan(&stored); err != nil {
		return false, nil, fmt.Errorf("approvals: reading the summary parts a notice shows: %w", err)
	}
	return true, readSummaryParts(stored), nil
}

// PendingAskedOfAtTx is PendingAskedOfInTx for a caller that holds only a
// transaction, such as the deal writer handing a request to a new owner.
func PendingAskedOfAtTx(ctx context.Context, tx pgx.Tx, id ids.ApprovalID, now time.Time) (bool, error) {
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
	return pendingDecidableInScopeAt(ctx, tx, id, ListInput{RequestedForReader: true}, now)
}
