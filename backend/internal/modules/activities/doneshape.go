// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Which activity a Done may name. Done says the reader dealt with the work a
// Worklist row stands for. So each row kind takes only the activity it is built
// from, and an id naming any other kind is refused.

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/platform/auth"
	"github.com/margince/margince/backend/internal/shared/apperrors"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/values"
)

// fieldRowID names a Worklist row's id, on the wire and in the audit image.
const fieldRowID = "row_id"

// directionOutbound is the mirror of directionInbound: the awaiting-reply lane
// reads it.
const directionOutbound = "outbound"

// activityShape reads the two columns every shape check judges.
func activityShape(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (kind string, direction *string, err error) {
	// Archived rows are excluded although the content gate before this already
	// refused them, so the read holds that on its own.
	err = tx.QueryRow(ctx, `SELECT kind, direction FROM activity WHERE id = $1 AND archived_at IS NULL`,
		id).Scan(&kind, &direction)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil, apperrors.ErrNotFound
	}
	if err != nil {
		return "", nil, fmt.Errorf("activities: reading what the activity is: %w", err)
	}
	return kind, direction, nil
}

// ensureInboundMessage is threadOf's shape check without the thread: Done is
// offered on threadless waiting rows too.
func ensureInboundMessage(ctx context.Context, tx pgx.Tx, id ids.ActivityID) error {
	kind, direction, err := activityShape(ctx, tx, id)
	if err != nil {
		return err
	}
	return inboundMessageShape(kind, direction)
}

// ensureOwnMessage admits the reader's side of an unanswered conversation: an
// email or channel message we sent.
func ensureOwnMessage(ctx context.Context, tx pgx.Tx, id ids.ActivityID) error {
	kind, direction, err := activityShape(ctx, tx, id)
	if err != nil {
		return err
	}
	if (kind != kindEmail && kind != kindMessage) || direction == nil || *direction != directionOutbound {
		return &values.ParseError{
			Field: fieldRowID, Code: "not_an_outbound_message",
			Message: "a follow-up row names a message we sent",
		}
	}
	return nil
}

// ensureMeeting admits a meeting, the activity a meeting follow-up row names.
func ensureMeeting(ctx context.Context, tx pgx.Tx, id ids.ActivityID) error {
	kind, _, err := activityShape(ctx, tx, id)
	if err != nil {
		return err
	}
	if kind != KindMeeting {
		return &values.ParseError{
			Field: fieldRowID, Code: codeNotAMeeting,
			Message: "a meeting follow-up row names a meeting",
		}
	}
	return nil
}

// sameStateWritesNothing makes a second Done on a message already done change
// nothing, so it records no second audit row and no second event.
func sameStateWritesNothing(state string) string {
	if state != stateHandled {
		return ""
	}
	return `
				WHERE activity_reader_state.state <> '` + stateHandled + `'`
}

// judgeInTx is judgeMessage's row gate, for a Worklist Done that already holds
// the object grant and one transaction for its whole batch.
func judgeInTx(ctx context.Context, tx pgx.Tx, id ids.ActivityID, capturedBy string, write judgement) error {
	if err := auth.EnsureActivityContentVisibleLive(ctx, tx, id.UUID); err != nil {
		return err
	}
	return write(ctx, tx, capturedBy)
}
