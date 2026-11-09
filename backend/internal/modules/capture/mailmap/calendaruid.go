// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package mailmap

import "strings"

// maxCalendarUIDLength bounds what a sender can make the row store. RFC 5545
// sets no limit; real providers stay far under this one.
const maxCalendarUIDLength = 255

// eventUID is the UID of the first VEVENT inside a VCALENDAR: the event an
// invitation names, and what an answer to it is addressed to. The calendar
// connector keys the same event by this UID, so the two meet on it.
func eventUID(body []byte) string {
	inEvent := false
	for _, line := range unfoldICalendar(body) {
		name, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		name, _, _ = strings.Cut(name, ";")
		switch {
		case strings.EqualFold(name, "BEGIN") && strings.EqualFold(strings.TrimSpace(value), "VEVENT"):
			inEvent = true
		case strings.EqualFold(name, "END") && strings.EqualFold(strings.TrimSpace(value), "VEVENT"):
			inEvent = false
		case inEvent && strings.EqualFold(name, "UID"):
			uid := strings.TrimSpace(value)
			if len(uid) > maxCalendarUIDLength {
				return ""
			}
			return uid
		}
	}
	return ""
}

// uid is the event the message's calendar parts name: the first part that
// names one. A disputed part names none, for the reason it names no method.
func (c calendarPart) uid() string {
	for _, part := range c.parts {
		if !part.disputed && part.uid != "" {
			return part.uid
		}
	}
	return ""
}

// CalendarUID is the iCalendar UID of the event this message's calendar part
// names, or "" when it names none.
func (m Message) CalendarUID() string { return m.calendarUID }
