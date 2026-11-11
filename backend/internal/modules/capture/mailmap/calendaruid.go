// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import (
	"strings"

	"github.com/margince/margince/backend/internal/shared/kernel/calendarevent"
)

// maxCalendarUIDLength bounds what a sender can make the row store. RFC 5545
// sets no limit; real providers stay far under this one.
const maxCalendarUIDLength = 255

// eventUID is the UID of the first VEVENT inside a VCALENDAR: the event an
// invitation names, and what an answer to it is addressed to. The calendar
// connector keys the same event by this UID, so the two meet on it.
func eventUID(body []byte) string {
	inEvent := false
	for _, line := range icalLines(body) {
		value := strings.TrimSpace(line.value)
		switch {
		case line.is("BEGIN") && strings.EqualFold(value, "VEVENT"):
			inEvent = true
		case line.is("END") && strings.EqualFold(value, "VEVENT"):
			inEvent = false
		case inEvent && line.is("UID"):
			if len(value) > maxCalendarUIDLength {
				return ""
			}
			return value
		}
	}
	return ""
}

// named is the first calendar part that names an event. A disputed part
// names none, for the reason it names no method.
func (c calendarPart) named() (partMethods, bool) {
	for _, part := range c.parts {
		if !part.disputed && part.uid != "" {
			return part, true
		}
	}
	return partMethods{}, false
}

// uid is the event the message's calendar parts name.
func (c calendarPart) uid() string {
	part, _ := c.named()
	return part.uid
}

// event is what the part naming the event says of it, or nil.
func (c calendarPart) event() *calendarevent.Event {
	part, _ := c.named()
	return part.event
}
