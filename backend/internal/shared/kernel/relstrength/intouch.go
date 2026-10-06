// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package relstrength

// Which activity says we are in touch with someone, for every reading that
// asks: the contact strength fold and the company relationship rating. Two
// readings that counted different things disagreed on one page about one
// account, so the meeting rule lives here once.

import (
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
)

// InTouchDays is how long since they last wrote or we last met before contact
// with an account no longer counts as current.
const InTouchDays = 30

// MeetingCountsSQL is the predicate a meeting row passes to count as contact:
// not canceled and not a no-show. A NULL status is a hand-logged meeting
// nobody tracks a status on, which is a real meeting. alias is a compile-time
// literal at every call site.
func MeetingCountsSQL(alias string) string {
	return "(" + alias + ".meeting_status IS NULL OR " + alias + ".meeting_status IN ('booked', 'held'))"
}

// InteractionCountsSQL is the whole test for an activity row that counts as
// contact: an interaction kind, and if it is a meeting, one that took place
// or still will.
func InteractionCountsSQL(alias string) string {
	return alias + ".kind IN " + InteractionKindSQLGroup() +
		" AND (" + alias + ".kind <> 'meeting' OR " + MeetingCountsSQL(alias) + ")"
}

// How an account's contact stands, by what made it so.
const (
	InTouchWrote  = "wrote"  // they wrote to us inside InTouchDays
	InTouchMet    = "met"    // we met them inside InTouchDays
	InTouchBooked = "booked" // quiet, but a meeting with them is booked ahead
	InTouchQuiet  = "quiet"  // no word and no meeting inside InTouchDays
	InTouchNever  = "never"  // no word and no meeting, ever
)

// InTouch is one account's contact reading. Days counts from the latest of
// their last message and our last meeting, and is zero when there was none.
type InTouch struct {
	Basis    string
	Days     int
	BookedAt *time.Time
}

// Current reports whether the reading keeps the account off "at risk".
func (r InTouch) Current() bool {
	return r.Basis == InTouchWrote || r.Basis == InTouchMet || r.Basis == InTouchBooked
}

// ReadInTouch folds the three dates every in-touch question rests on. A
// meeting at or before now has been held; one after it is booked ahead and
// counts only as nextMeeting, so a caller passes each to its own argument.
func ReadInTouch(lastInbound, lastMeeting, nextMeeting *time.Time, now time.Time) InTouch {
	latest, basis := lastInbound, InTouchWrote
	if lastMeeting != nil && !lastMeeting.After(now) && (latest == nil || lastMeeting.After(*latest)) {
		latest, basis = lastMeeting, InTouchMet
	}
	var reading InTouch
	if latest != nil {
		reading.Days = elapsed.Days(*latest, now)
	}
	if nextMeeting != nil && nextMeeting.After(now) {
		reading.BookedAt = nextMeeting
	}
	switch {
	case latest != nil && reading.Days <= InTouchDays:
		reading.Basis = basis
	case reading.BookedAt != nil:
		reading.Basis = InTouchBooked
	case latest != nil:
		reading.Basis = InTouchQuiet
	default:
		reading.Basis = InTouchNever
	}
	return reading
}
