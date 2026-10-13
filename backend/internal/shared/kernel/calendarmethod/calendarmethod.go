// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package calendarmethod holds the RFC 5546 iCalendar methods. The mail parser
// that stores one and the owed predicate that reads one agree here on which
// methods answer an invitation.
//
// Stdlib only, which the shared tier requires.
package calendarmethod

import "strings"

// known are the methods RFC 5546 defines; activity.calendar_method's CHECK
// holds the same eight.
var known = map[string]bool{
	"PUBLISH": true, "REQUEST": true, "REPLY": true, "ADD": true,
	"CANCEL": true, "REFRESH": true, "COUNTER": true, "DECLINECOUNTER": true,
}

// Known normalises a declared method, or returns "" for one the RFC does not
// define, so a sender cannot store a word of their choosing.
func Known(value string) string {
	method := strings.ToUpper(strings.TrimSpace(value))
	if !known[method] {
		return ""
	}
	return method
}

// IsResponse reports an attendee answering the organizer rather than asking.
func IsResponse(method string) bool {
	switch method {
	case "REPLY", "COUNTER", "DECLINECOUNTER":
		return true
	}
	return false
}
