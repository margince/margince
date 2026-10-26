// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"fmt"
	"time"
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
	return fmt.Sprintf(`least(coalesce((SELECT horizon_days FROM activity_waiting_horizon WHERE measured_at > $%d), $%d), $%d)`, arg(asOf.Add(-WaitingHorizonStaleAfter)), ceiling, ceiling)
}
