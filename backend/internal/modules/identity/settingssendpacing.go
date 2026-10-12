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
var SendRateLimit = settings.Define[int]("installation.send_rate_limit", installationSettingsObject, "update", 30,
	settings.Between("messages", 1, 1000)).MachineryApplied()

// SendRateWindowSeconds is the window the send rate is counted over.
var SendRateWindowSeconds = settings.Define[int]("installation.send_rate_window_seconds", installationSettingsObject, "update", 60,
	settings.Between("seconds", 10, 3600)).MachineryApplied()

// SendMaxAgeHours is how long a held-back delivery may wait before it parks
// with a reason. A day by default: past that a message nobody could send has
// stopped being news, and somebody should see why it did not go.
var SendMaxAgeHours = settings.Define[int]("installation.send_max_age_hours", installationSettingsObject, "update", 24,
	settings.Between("hours", 1, 168)).MachineryApplied()

func sendPacingDefinitions() []settings.Definition {
	return []settings.Definition{SendRateLimit, SendRateWindowSeconds, SendMaxAgeHours}
}
