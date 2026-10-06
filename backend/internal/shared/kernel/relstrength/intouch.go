// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package relstrength

// Which activity says we are in touch with someone, for every reading that
// asks: the contact strength fold and the company relationship rating. Two
// readings that counted different things disagreed on one page about one
// account, so the meeting rule lives here once.

import (
	"slices"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
)

// InTouchDays is how long since they last wrote or we last met before contact
// with an account no longer counts as current.
const InTouchDays = 30

// countingMeetingStatuses are the statuses of a meeting that took place or is
// booked. An untracked meeting (no status) counts too; canceled and no_show
// never do. Both the SQL and the Go predicate below render from this list.
var countingMeetingStatuses = []string{"booked", "held"}

// MeetingCountsSQL is the predicate a meeting row passes to count toward
// contact strength: not canceled and not a no-show, whenever it is. alias, and
// now in the two below, are compile-time literals or bind placeholders.
func MeetingCountsSQL(alias string) string {
	return "(" + alias + ".meeting_status IS NULL OR " + alias + ".meeting_status IN (" +
		sqlList(countingMeetingStatuses) + "))"
}

// MeetingStatusCounts is MeetingCountsSQL for a reader holding the row in Go;
// status is "" when the meeting has none.
func MeetingStatusCounts(status string) bool {
	return status == "" || slices.Contains(countingMeetingStatuses, status)
}

// InteractionCounts is InteractionCountsSQL for a reader holding the row in Go.
func InteractionCounts(kind, meetingStatus string) bool {
	return IsInteractionKind(kind) && (kind != kindMeeting || MeetingStatusCounts(meetingStatus))
}

// MeetingTookPlaceSQL is a meeting that counts as contact we had: not called
// off, and already started. A `booked` row stays booked after it happens,
// because few users ever mark a meeting held.
func MeetingTookPlaceSQL(alias, now string) string {
	return "(" + MeetingCountsSQL(alias) + " AND " + alias + ".occurred_at <= " + now + ")"
}

// MeetingAheadSQL is a meeting still to come that nobody called off. A `held`
// row dated ahead is a data error, not a booking.
func MeetingAheadSQL(alias, now string) string {
	return "((" + alias + ".meeting_status IS NULL OR " + alias + ".meeting_status = 'booked') AND " +
		alias + ".occurred_at > " + now + ")"
}

// InteractionCountsSQL is the whole test for an activity row that counts
// toward contact strength: an interaction kind, and if it is a meeting, one
// nobody called off.
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
// message or meeting dated after now has not happened, so it never counts as
// contact we had; a meeting ahead counts only as nextMeeting.
func ReadInTouch(lastInbound, lastMeeting, nextMeeting *time.Time, now time.Time) InTouch {
	latest, basis := lastInbound, InTouchWrote
	if latest != nil && latest.After(now) {
		latest = nil
	}
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
