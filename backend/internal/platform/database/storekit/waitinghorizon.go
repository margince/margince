// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// DefaultWaitingHorizonDays applies until response history supports a current
// measurement. It is also the ceiling no measurement may exceed. An automatic
// wait older than 30 days is record information, not current work.
const DefaultWaitingHorizonDays = 30

// WaitingHorizonStaleAfter bounds how long an old measurement may govern live work.
const WaitingHorizonStaleAfter = 24 * time.Hour

// WaitingHorizonSQL reads the stored measurement without recomputing it for each
// task. It clamps to the ceiling, so a horizon stored under an older, longer
// rule cannot keep old waits current.
func WaitingHorizonSQL(asOf time.Time, arg func(any) int) string {
	ceiling := arg(DefaultWaitingHorizonDays)
	return fmt.Sprintf(`least(coalesce((SELECT (value->>'horizon_days')::int FROM installation_state
		WHERE key = '%s' AND (value->>'measured_at')::timestamptz > $%d), $%d), $%d)`,
		StateActivityWaitingHorizon, arg(asOf.Add(-WaitingHorizonStaleAfter)), ceiling, ceiling)
}

// waitingHorizon is the stored measurement. WaitingHorizonSQL reads these two
// names inside SQL, so they change together with it.
type waitingHorizon struct {
	Days       int       `json:"horizon_days"`
	MeasuredAt time.Time `json:"measured_at"`
}

// StoreWaitingHorizonTx stores what the worker pass measured. A horizon under
// one day is refused: it would end each wait on the day the mail went out.
func StoreWaitingHorizonTx(ctx context.Context, tx pgx.Tx, days int, measuredAt time.Time) error {
	if days <= 0 {
		return fmt.Errorf("storekit: a waiting horizon of %d days is not a measurement", days)
	}
	// Cut to the microsecond a timestamptz holds. The reader casts this text,
	// and a cast rounds, which could move the instant past the reader's bound.
	return StoreInstallationStateTx(ctx, tx, StateActivityWaitingHorizon,
		waitingHorizon{Days: days, MeasuredAt: measuredAt.Truncate(time.Microsecond)})
}
