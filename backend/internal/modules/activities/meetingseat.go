// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

// Who a calendar verb is entitled to act on, when it found the row by somebody
// else's key.

import (
	"context"

	"github.com/jackc/pgx/v5"

	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// SeatStanding reports whether the seat behind this call already holds the
// activity — it captured the row, imported it, or sits on it as a participant.
//
// A seam because the answer lives in `capture` (the import rows, and the
// provenance stamp its connectors write) while the decision lives here, under
// the row's own lock, and neither module may import the other. Compose injects
// it — the same shape AddressProver travels on.
//
// Nil is a caller that carries its own provenance and needs no guard: the RSVP
// backfill re-reads a row's own stored original and runs with no seat at all,
// so a guard reaching it would answer "not yours" about a row it took the
// question from.
type SeatStanding func(ctx context.Context, tx pgx.Tx, id ids.ActivityID) (bool, error)
