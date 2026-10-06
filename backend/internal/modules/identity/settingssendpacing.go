// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// How fast one mailbox may send, and how long a delivery held back by that
// pace may wait before it stops with a reason. The send worker reads them at
// the start of every delivery, so a change applies to the next send.

import (
	"github.com/margince/margince/backend/internal/platform/settings"
)

var (
	// The rate is a burst bound, not a quota: Gmail enforces its own daily cap
	// and throttles an account that bursts past it, so this keeps a legitimate
	// run of sends from costing a user their mailbox's standing.
	SendRateLimit         = boundedInt("installation.send_rate_limit", "messages", 30, 1, 1000)
	SendRateWindowSeconds = boundedInt("installation.send_rate_window_seconds", "seconds", 60, 10, 3600)
	// A day by default: past that a message nobody could send has stopped
	// being news, and somebody should see why it did not go.
	SendMaxAgeHours = boundedInt("installation.send_max_age_hours", "hours", 24, 1, 168)
)

func sendPacingDefinitions() []settings.Definition {
	return []settings.Definition{SendRateLimit, SendRateWindowSeconds, SendMaxAgeHours}
}
