// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package capture

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// memberChatPostureTx gives a message on one member's own chat account the
// posture that member's mailboxes ask for. Their chats are then shared or held
// as their mail is, and the recompute treats both alike.
//
// A chat transport has no capture_connection row for a posture of its own, so
// the seat's strictest live mailbox decides. A seat with none holds the message.
//
// A classified chat stays held: the confidentiality ledger judges a thread once,
// and a chat is one long thread.
func memberChatPostureTx(ctx context.Context, tx pgx.Tx, decision birthDecision) (birthDecision, error) {
	posture, err := seatStrictestPostureTx(ctx, tx, actorUserID(ctx))
	if err != nil {
		return birthDecision{}, err
	}
	if posture == PostureShared {
		return decision, nil
	}
	decision = decision.hold(audienceReasonPosture)
	decision.posture = posture
	return decision, nil
}

// seatStrictestPostureTx is the strictest mail posture across a seat's live
// mailboxes: held, then classified, then shared. No mailbox answers held.
func seatStrictestPostureTx(ctx context.Context, tx pgx.Tx, seat ids.UUID) (string, error) {
	rows, err := tx.Query(ctx, `SELECT mail_posture FROM capture_connection WHERE user_id = $1 AND archived_at IS NULL`, seat)
	if err != nil {
		return "", fmt.Errorf("capture: reading the seat's mailbox postures: %w", err)
	}
	postures, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return "", fmt.Errorf("capture: reading the seat's mailbox postures: %w", err)
	}
	strictest := PostureShared
	for _, posture := range postures {
		switch {
		case posture == PostureHeld:
			return PostureHeld, nil
		case posture == PostureClassified:
			strictest = PostureClassified
		}
	}
	if len(postures) == 0 {
		return PostureHeld, nil
	}
	return strictest, nil
}
