// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package meetingmap

import "github.com/margince/margince/backend/internal/shared/ports/connector"

// crossDoorIdentity names the occurrence by its series and its slot in it. A
// moved occurrence keeps the identity every other door knows it by.
func (m Meeting) crossDoorIdentity() connector.CrossDoorIdentity {
	occurrence := m.seriesSlot
	if occurrence.IsZero() {
		occurrence = m.occurredAt
	}
	return connector.CrossDoorIdentity{Series: m.icalUID, Occurrence: occurrence, AllDay: m.allDay}
}
