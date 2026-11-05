// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package calendarmethod holds the RFC 5546 iCalendar methods. The mail parser
// that stores one and the owed predicate that reads one agree here on which
// methods answer an invitation.
//
// Stdlib only, which the shared tier requires.
package calendarmethod

import (
	"slices"
	"strings"
)

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

// responses answer the organizer; invitations come from the organizer. Every
// known method is in one of the two, and none is in both.
var (
	responses   = []string{"REPLY", "COUNTER", "DECLINECOUNTER"}
	invitations = []string{"REQUEST", "ADD", "CANCEL", "PUBLISH", "REFRESH"}
)

// Responses lists the methods IsResponse admits, for a reader that spells them
// in SQL.
func Responses() []string { return slices.Clone(responses) }

// Invitations lists the methods IsInvitation admits, for a reader that spells
// them in SQL.
func Invitations() []string { return slices.Clone(invitations) }

// IsResponse reports an attendee answering the organizer rather than asking.
func IsResponse(method string) bool {
	return slices.Contains(responses, method)
}

// IsInvitation reports the organizer sending an invitation, an update or a
// cancellation. Such a mail asks for an RSVP, never for a written reply.
func IsInvitation(method string) bool {
	return slices.Contains(invitations, method)
}
