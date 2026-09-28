// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/connector"
)

// CancelMeetingByIDTx marks one known meeting cancelled — the row the calendar
// event resolved to through its cross-door identity rather than its natural key.
//
// A calendar event matched onto a meeting another door filed (a HubSpot import,
// a colleague's calendar) never lands a row under the calendar's own key: capture
// resolves it onto the incumbent and writes nothing of its own. The natural-key
// cancel then finds nothing, and the imported meeting stayed booked after the
// calendar called it off. The caller resolved WHICH row through the same
// bindable identity check capture matched it with, so this cancels that row and
// no other.
//
// The same rules as CancelCapturedMeetingTx, under the same lock: only a
// meeting, only a live and unrestricted one, only an unanswered one. `key` is
// the calendar event's own natural key, carried as the transition's idempotency
// key, so a resynced calendar still records one cancellation.
func CancelMeetingByIDTx(
	ctx context.Context, tx pgx.Tx, id ids.ActivityID, key connector.NaturalKey, at time.Time,
) (bool, error) {
	if key.SourceSystem == "" || key.SourceID == "" {
		return false, errors.New("activities: cancelling a matched meeting needs the calendar's natural key")
	}
	var status *string
	err := tx.QueryRow(ctx, `
		SELECT meeting_status FROM activity
		 WHERE id = $1
		   AND kind = 'meeting' AND restricted_at IS NULL AND archived_at IS NULL
		 FOR UPDATE`, id).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("activities: reading the matched meeting being cancelled: %w", err)
	}
	if !cancellableFrom(status) {
		return false, nil
	}
	if err := writeMeetingCancellationTx(ctx, tx, id, status, string(crmcontracts.ActivityMeetingStatusCanceled), at, key); err != nil {
		return false, err
	}
	return true, nil
}
