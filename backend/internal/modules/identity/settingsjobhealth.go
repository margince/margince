// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package identity

// How far back the maintenance banner looks when it calls dead work a problem.

import (
	"fmt"

	"github.com/margince/margince/backend/internal/platform/settings"
)

// DeadWorkBannerHours is the age bound on the count that gets the red treatment.
//
// The full count of discarded and cancelled rows is not wrong — that work did
// not happen — but River retains those rows for seven days, so a transient
// outage that ended an hour ago goes on asserting that hundreds of jobs need a
// hand for the rest of the week. What was wrong was presenting a settled history
// as a live call to action, and the fix is which number gets the alarm rather
// than the arithmetic.
//
// TWENTY-FOUR HOURS by default. Short enough that a finished outage clears
// itself by the next working day; long enough that something which died
// overnight is still on the banner in the morning, which is when an operator
// first looks. An hour was the tempting number and it is too short — a
// Friday-evening failure would be gone by Monday and nobody would ever see it.
//
// A SETTING because an installation's rhythm is its own: a team that watches
// this hourly wants a shorter window than one that reads it on Monday, and a
// constant here is a number somebody has to argue with in code.
//
// The upper bound is River's own retention. A window longer than the rows live
// is a window that silently means "everything", which is the state this exists
// to leave.
var DeadWorkBannerHours = settings.Define[int](
	"installation.dead_work_banner_hours",
	installationSettingsObject,
	"update",
	24,
	func(hours int) error {
		if hours < 1 || hours > riverRetentionHours {
			return fmt.Errorf("the dead-work banner looks back 1..%d hours, not %d", riverRetentionHours, hours)
		}
		return nil
	},
).AsInstallationIdentity()

// riverRetentionHours is how long River's job cleaner keeps a discarded or
// cancelled row at its defaults — seven days. It bounds the setting above
// because a window past it cannot narrow anything: every row still there is
// inside it, and the banner would be back to counting the week.
const riverRetentionHours = 7 * 24
