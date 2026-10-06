// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// How fast one mailbox may send, and how long a delivery held back by that
// pace may wait before it stops with a reason. The send worker reads them at
// the start of every delivery, so a change applies to the next send.

import (
	"github.com/margince/margince/backend/internal/platform/settings"
)

// SendRateLimit is how many messages one mailbox may send per window. A burst
// bound, not a quota: Gmail enforces its own daily cap and throttles an account
// that bursts past it, so this keeps a legitimate run of sends from costing a
// user their mailbox's standing.
var SendRateLimit = boundedInt("installation.send_rate_limit", "messages", 30, 1, 1000)

// SendRateWindowSeconds is the window the send rate is counted over.
var SendRateWindowSeconds = boundedInt("installation.send_rate_window_seconds", "seconds", 60, 10, 3600)

// SendMaxAgeHours is how long a held-back delivery may wait before it parks
// with a reason. A day by default: past that a message nobody could send has
// stopped being news, and somebody should see why it did not go.
var SendMaxAgeHours = boundedInt("installation.send_max_age_hours", "hours", 24, 1, 168)

func sendPacingDefinitions() []settings.Definition {
	return []settings.Definition{SendRateLimit, SendRateWindowSeconds, SendMaxAgeHours}
}
