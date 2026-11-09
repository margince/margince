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

// SeriesIdentityOf reads the identity of one stored occurrence of a series,
// and reports false for an event that names no slot in one. A slot is the
// same in every version a calendar delivers. So a stored original states it
// even when the occurrence moved after it was stored. A one-off's start does
// not: the original may predate a reschedule.
func SeriesIdentityOf(raw []byte, decode Decode) (connector.CrossDoorIdentity, bool, error) {
	ev, err := decode(raw, "")
	if err != nil {
		return connector.CrossDoorIdentity{}, false, err
	}
	m := Classify(ev, "")
	identity := m.crossDoorIdentity()
	return identity, !m.seriesSlot.IsZero() && identity.Stated(), nil
}
