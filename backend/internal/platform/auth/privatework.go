// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package auth

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// SeatMayHoldPrivateWorkTx reports whether seat may hold work read out of
// source, a message on a thread private to privateTo. A shared thread (nil)
// admits every seat. A private one admits its owner. It also admits a seat that
// may open that one message on evidence it did not write. That evidence is its
// sender stamp, its own mailbox's copy, or an attested place on the party list. Links are not
// walked: on a private thread every link is the owner's alone. It asks the read
// gate's own arms and needs no sign-in, so an invited seat can be asked.
func SeatMayHoldPrivateWorkTx(ctx context.Context, tx pgx.Tx, source ids.UUID, privateTo *ids.UUID, seat ids.UUID) (bool, error) {
	if privateTo == nil || *privateTo == seat {
		return true, nil
	}
	if seat.IsZero() {
		return false, nil
	}
	holder := principal.Principal{Type: principal.PrincipalHuman, UserID: seat}
	var args []any
	arg := func(value any) int { args = append(args, value); return len(args) }
	row := arg(source)
	query := fmt.Sprintf(`SELECT EXISTS (SELECT 1 FROM activity a
		 WHERE a.id = $%d AND a.archived_at IS NULL AND %s AND %s AND %s)`,
		row, ActivityAvailableClause("a"), activityAttendanceArm(holder, "a", arg), activityAudienceArm(holder, "a", arg))
	var may bool
	if err := tx.QueryRow(ctx, query, args...).Scan(&may); err != nil {
		return false, fmt.Errorf("auth: reading whether a colleague may hold work from private mail: %w", err)
	}
	return may, nil
}
