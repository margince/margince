// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

// Package calendarevent holds what one calendar mail said about its event, as
// the mail parser stores it and the reader compares two of them. Both sides
// agree here on the stored shape and on what counts as a change.
//
// Stdlib only, which the shared tier requires.
package calendarevent

import (
	"slices"
	"strings"
	"time"
)

// MaxAttendees bounds the guests one event stores. A larger list is cut and
// says so, and a cut list is never compared: a guest past the cut is unknown,
// not removed.
const MaxAttendees = 100

// Event is the first VEVENT of a calendar part, in the words a reader needs.
type Event struct {
	// Sequence is the organizer's revision count: 0 for the first send, higher
	// for each update (RFC 5545 SEQUENCE).
	Sequence int    `json:"sequence"`
	Title    string `json:"title,omitempty"`
	Location string `json:"location,omitempty"`
	// Start is nil when the part named no start or a zone nothing resolves.
	Start  *time.Time `json:"start,omitempty"`
	End    *time.Time `json:"end,omitempty"`
	AllDay bool       `json:"all_day,omitempty"`
	// Canceled is STATUS:CANCELLED, which some calendars send under REQUEST.
	Canceled bool `json:"canceled,omitempty"`
	// Occurrence is the RECURRENCE-ID: one meeting of a series, as written.
	Occurrence   string     `json:"occurrence,omitempty"`
	Organizer    string     `json:"organizer,omitempty"`
	Attendees    []Attendee `json:"attendees,omitempty"`
	AttendeesCut bool       `json:"attendees_cut,omitempty"`
}

// Attendee is one guest: their address, the name the calendar gave, and their
// answer where the part states one.
type Attendee struct {
	Email  string `json:"email"`
	Name   string `json:"name,omitempty"`
	Answer string `json:"answer,omitempty"`
}

// Label is how a guest is named to a reader: the calendar's name, else the
// address.
func (a Attendee) Label() string {
	if name := strings.TrimSpace(a.Name); name != "" && !strings.EqualFold(name, a.Email) {
		return name
	}
	return a.Email
}

// Answers are the guest answers a reader is told, from RFC 5545 PARTSTAT.
const (
	AnswerAccepted  = "accepted"
	AnswerTentative = "tentative"
	AnswerDeclined  = "declined"
)

// AnswerOf reads a PARTSTAT, or "" for one that is not an answer.
func AnswerOf(partstat string) string {
	switch strings.ToUpper(strings.TrimSpace(partstat)) {
	case "ACCEPTED":
		return AnswerAccepted
	case "TENTATIVE":
		return AnswerTentative
	case "DECLINED":
		return AnswerDeclined
	}
	return ""
}

// Difference is what changed from one send of an event to a later one. Each
// send states the whole event, so an empty place is a place taken away. A
// time or guest list either side does not know is never reported as changed.
type Difference struct {
	Time     bool
	Location bool
	Title    bool
	Added    []Attendee
	Removed  []Attendee
}

// Any says whether anything changed.
func (d Difference) Any() bool {
	return d.Time || d.Location || d.Title || len(d.Added) > 0 || len(d.Removed) > 0
}

// Compare is what after changed from before.
func Compare(before, after Event) Difference {
	var out Difference
	if before.Start != nil && after.Start != nil {
		out.Time = !before.Start.Equal(*after.Start) || before.AllDay != after.AllDay ||
			(before.End != nil && after.End != nil && !before.End.Equal(*after.End))
	}
	out.Location = !strings.EqualFold(strings.TrimSpace(before.Location), strings.TrimSpace(after.Location))
	out.Title = strings.TrimSpace(before.Title) != strings.TrimSpace(after.Title)
	if !before.AttendeesCut && !after.AttendeesCut && len(before.Attendees) > 0 && len(after.Attendees) > 0 {
		out.Added = missingFrom(after.Attendees, before.Attendees)
		out.Removed = missingFrom(before.Attendees, after.Attendees)
	}
	return out
}

// missingFrom lists the guests of from whose address other does not hold.
func missingFrom(from, other []Attendee) []Attendee {
	var out []Attendee
	for _, guest := range from {
		if !slices.ContainsFunc(other, func(o Attendee) bool { return strings.EqualFold(o.Email, guest.Email) }) {
			out = append(out, guest)
		}
	}
	return out
}
