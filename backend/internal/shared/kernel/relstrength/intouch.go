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

	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
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
// automatic is the row's Evidence.Automatic, empty when a human wrote it.
func InteractionCounts(kind, meetingStatus string, automatic AutomaticKind) bool {
	return IsInteractionKind(kind) && (kind != kindMeeting || MeetingStatusCounts(meetingStatus)) && automatic == ""
}

// MeetingTookPlaceSQL is a meeting that counts as contact we had: not called
// off, and already started (occurredday.BegunSQL). A `booked` row stays booked
// after it happens, because few users ever mark a meeting held.
func MeetingTookPlaceSQL(alias, now string) string {
	return "(" + MeetingCountsSQL(alias) + " AND " + occurredday.BegunSQL(alias, now) + ")"
}

// MeetingAheadSQL is a meeting still to come (occurredday.AheadSQL) that nobody
// called off. A `held` row dated ahead is a data error, not a booking.
func MeetingAheadSQL(alias, now string) string {
	return "((" + alias + ".meeting_status IS NULL OR " + alias + ".meeting_status = 'booked') AND " +
		occurredday.AheadSQL(alias, now) + ")"
}

// NotCalledOffSQL admits any activity row except a meeting that was canceled or
// a no-show. last_activity_of_deal carries this text verbatim.
func NotCalledOffSQL(alias string) string {
	return "(" + alias + ".kind <> '" + kindMeeting + "' OR " + MeetingCountsSQL(alias) + ")"
}

// lastActivityKinds are the kinds that move a record's last-activity clock. A
// task is intent, not contact, whether a member or an agent filed it. A note
// moves the clock but never scores, so this is not interactionKinds.
var lastActivityKinds = []string{kindEmail, kindMessage, kindMeeting, kindCall, "note"}

// LastActivityKinds lists lastActivityKinds for the gate that holds the
// frontend's copy of the set to this one.
func LastActivityKinds() []string {
	return slices.Clone(lastActivityKinds)
}

// LastActivityKindSQL admits an activity row whose kind moves the clock.
func LastActivityKindSQL(alias string) string {
	return alias + ".kind IN (" + sqlList(lastActivityKinds) + ")"
}

// LastActivityCountsSQL admits an activity row to a record's last-activity
// clock: a kind that moves it, not a called-off meeting, and already happened
// at now (occurredday.BegunSQL). The last_activity_of_* functions carry its
// text rendered with now().
func LastActivityCountsSQL(alias, now string) string {
	return "(" + LastActivityKindSQL(alias) + " AND " + NotCalledOffSQL(alias) + " AND " +
		occurredday.BegunSQL(alias, now) + ")"
}

// InteractionCountsSQL is the whole test for an activity row that counts as an
// exchange. It needs an interaction kind and a meeting nobody called off. It
// also needs a row a human wrote (WrittenByHumanSQL).
func InteractionCountsSQL(alias string) string {
	return alias + ".kind IN " + InteractionKindSQLGroup() + " AND " + NotCalledOffSQL(alias) +
		" AND " + WrittenByHumanSQL(alias)
}

// InteractionRolesSQL lists the participant roles that make a party part of an
// exchange. The graph fold and the seats-without-mailbox notice both read it,
// so a role added for one cannot be missed by the other.
const InteractionRolesSQL = `('from','to','cc','bcc','attendee','organizer')`

// WorkspaceAudienceSQL admits only an activity every member may read. A limited
// message (audience participants|selected) is content, so anything derived for
// all readers from its participants or timing would disclose who talked to whom.
func WorkspaceAudienceSQL(alias string) string {
	return alias + ".audience = 'workspace'"
}

// How an account's contact stands, by what made it so.
const (
	InTouchWrote  = "wrote"  // they wrote to us inside InTouchDays
	InTouchMet    = "met"    // we met them inside InTouchDays
	InTouchBooked = "booked" // quiet, but a meeting with them is booked ahead
	InTouchQuiet  = "quiet"  // no word and no meeting inside InTouchDays
	InTouchNever  = "never"  // no word and no meeting, ever
)

// Dated is when an activity happened, and whether that names only a date: a
// day entry's stored anchor. A nil At is no activity.
type Dated struct {
	At       *time.Time
	DateOnly bool
}

// begun reports whether d names an activity that has happened at now.
func (d Dated) begun(now time.Time, loc *time.Location) bool {
	return d.At != nil && occurredday.Begun(*d.At, d.DateOnly, now, loc)
}

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
// message or meeting that has not happened (occurredday.Begun) never counts as
// contact we had; a meeting ahead counts only as nextMeeting. Days are counted
// by the calendar of loc, the zone the reader sees the dates in.
func ReadInTouch(lastInbound, lastMeeting Dated, nextMeeting *time.Time, now time.Time, loc *time.Location) InTouch {
	latest, basis := lastInbound, InTouchWrote
	if !latest.begun(now, loc) {
		latest = Dated{}
	}
	if lastMeeting.begun(now, loc) && (latest.At == nil || lastMeeting.At.After(*latest.At)) {
		latest, basis = lastMeeting, InTouchMet
	}
	var reading InTouch
	if latest.At != nil {
		reading.Days = occurredday.DaysSince(*latest.At, latest.DateOnly, now, loc)
	}
	if nextMeeting != nil && nextMeeting.After(now) {
		reading.BookedAt = nextMeeting
	}
	switch {
	case latest.At != nil && reading.Days <= InTouchDays:
		reading.Basis = basis
	case reading.BookedAt != nil:
		reading.Basis = InTouchBooked
	case latest.At != nil:
		reading.Basis = InTouchQuiet
	default:
		reading.Basis = InTouchNever
	}
	return reading
}
