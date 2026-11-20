// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package consent

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// MessageAlreadySentError answers a Send anyway on a review whose message has
// gone, from another tab or an earlier press. It is a 409 with its own code,
// so the sender reads that the message went rather than that nothing exists.
type MessageAlreadySentError struct{}

func (*MessageAlreadySentError) Error() string { return "this message was already sent" }

// MessageFault names the condition for the client.
func (*MessageAlreadySentError) MessageFault() (code, message string) {
	return "already_sent", "this message was already sent; there is nothing left to decide"
}

// Unwrap makes the answer a conflict.
func (*MessageAlreadySentError) Unwrap() error { return apperrors.ErrConflict }

// SentInstead turns a not-found on a Send anyway into MessageAlreadySentError
// when the review was resolved by its message going out. Only the review's
// sender learns that; anyone else keeps the not-found.
func (s *Store) SentInstead(ctx context.Context, reviewID ids.UUID, cause error) error {
	seat := initiatingSeat(ctx)
	if !errors.Is(cause, apperrors.ErrNotFound) || seat.IsZero() {
		return cause
	}
	var sent bool
	if err := s.db.Tx(ctx, func(tx pgx.Tx) error {
		return tx.QueryRow(ctx, `
			SELECT EXISTS (SELECT 1 FROM communication_review
			                WHERE id = $1 AND state = $2 AND initiated_by = $3)`,
			reviewID, ReviewResolved, seat).Scan(&sent)
	}); err != nil {
		return fmt.Errorf("consent: asking whether this review's message went: %w", err)
	}
	if sent {
		return &MessageAlreadySentError{}
	}
	return cause
}
