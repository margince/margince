// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// How fast one mailbox may send, and how long a delivery held back by that
// pace may wait before it stops with a reason. The send worker applies them,
// with no session to gate against, at the start of every delivery, so a change
// applies to the next send.

import (
	"github.com/margince/margince/backend/internal/platform/config"
)

// SendRateLimit is how many messages one mailbox may send per window. A burst
// bound, not a quota: Gmail enforces its own daily cap and throttles an account
// that bursts past it, so this keeps a legitimate run of sends from costing a
// user their mailbox's standing.
var SendRateLimit = config.DefineStored(config.Spec[int]{
	Key:     "installation.send_rate_limit",
	Env:     "MARGINCE_INSTALLATION_SEND_RATE_LIMIT",
	Type:    config.Int,
	Default: config.Some(30),
	Min:     config.Some(1),
	Max:     config.Some(1000),
	Doc:     "Messages one mailbox may send per send-rate window.",
}, config.Store{RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, MachineryApplied: true})

// SendRateWindowSeconds is the window the send rate is counted over.
var SendRateWindowSeconds = config.DefineStored(config.Spec[int]{
	Key:     "installation.send_rate_window_seconds",
	Env:     "MARGINCE_INSTALLATION_SEND_RATE_WINDOW_SECONDS",
	Type:    config.Int,
	Default: config.Some(60),
	Min:     config.Some(10),
	Max:     config.Some(3600),
	Doc:     "Seconds in the window the send rate is counted over.",
}, config.Store{RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, MachineryApplied: true})

// SendMaxAgeHours is how long a held-back delivery may wait before it parks
// with a reason. A day by default: past that a message nobody could send has
// stopped being news, and somebody should see why it did not go.
var SendMaxAgeHours = config.DefineStored(config.Spec[int]{
	Key:     "installation.send_max_age_hours",
	Env:     "MARGINCE_INSTALLATION_SEND_MAX_AGE_HOURS",
	Type:    config.Int,
	Default: config.Some(24),
	Min:     config.Some(1),
	Max:     config.Some(168),
	Doc:     "Hours a delivery held back by the send rate may wait before it stops with a reason.",
}, config.Store{RBACObject: installationSettingsObject, AuditVerb: installationAuditVerb, MachineryApplied: true})

func sendPacingSettings() []config.Any {
	return []config.Any{SendRateLimit, SendRateWindowSeconds, SendMaxAgeHours}
}
