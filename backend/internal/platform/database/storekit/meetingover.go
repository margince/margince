// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package storekit

import (
	"fmt"

	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// MeetingIsOverSQL is "this meeting is finished" for the meeting under alias,
// judged at the placeholder asOf. Over means ENDED (occurredday.EndSQL), not
// started, and a meeting that will never happen counts as over: archived,
// canceled or a no-show. The waiting queue, the brief's snooze-lift and the
// deal sweeps ask it here.
func MeetingIsOverSQL(alias, asOf string) string {
	return fmt.Sprintf(`(%[1]s.archived_at IS NOT NULL
		OR %[1]s.meeting_status IN ('canceled', 'no_show')
		OR %[2]s <= %[3]s)`, alias, occurredday.EndSQL(alias), asOf)
}
