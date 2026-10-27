// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The meetings lane's seam over the activities store, and the two questions it
// answers about a booked meeting: what to call it, and whether anybody has
// prepared it.

import (
	"context"
	"strings"

	"github.com/margince/margince/backend/internal/compose/attention"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// attentionMeetings reads today's remaining meetings through the activities
// store, the same gated list every other activity surface reads.
//
// Window, status and order are all SQL predicates, so the limit cuts a list
// that is already the lane. The day meetings come first, as a calendar's
// all-day row, then the soonest open timed one. Filtering or sorting after the
// limit let canceled rows or later meetings hide the next one to prepare.
type attentionMeetings struct{ store *activities.Store }

// soonestFirst is the store's ascending order on the meeting's start.
var soonestFirst = "occurred_at"

func (m attentionMeetings) Today(
	ctx context.Context, window attention.MeetingWindow, limit int,
	scope attention.TaskScope, owner ids.UUID,
) ([]attention.Meeting, error) {
	days := attention.MeetingWindow{From: window.From, Until: window.From, DaysFrom: window.DaysFrom, DaysUntil: window.DaysUntil}
	rows, err := m.openIn(ctx, days, limit, scope, owner)
	if err != nil {
		return nil, err
	}
	if len(rows) < limit {
		timed := attention.MeetingWindow{From: window.From, Until: window.Until, DaysFrom: window.DaysUntil, DaysUntil: window.DaysUntil}
		hours, err := m.openIn(ctx, timed, limit-len(rows), scope, owner)
		if err != nil {
			return nil, err
		}
		rows = append(rows, hours...)
	}
	return m.named(ctx, rows)
}

// openIn reads the open meetings of one window, soonest first, up to limit.
func (m attentionMeetings) openIn(
	ctx context.Context, window attention.MeetingWindow, limit int,
	scope attention.TaskScope, owner ids.UUID,
) ([]crmcontracts.Activity, error) {
	kind := string(crmcontracts.ActivityKindMeeting)
	in := activities.ListActivitiesInput{
		Kind: &kind, OccurredAfter: &window.From, OccurredBefore: &window.Until, DayDates: dayDatesOf(window), Limit: &limit,
		OpenStatusOnly: true, Sort: &soonestFirst, ReadableOnly: true,
	}
	if !applyMeetingScope(ctx, &in, scope, owner) {
		return nil, nil
	}
	rows, _, err := m.store.ListActivities(ctx, in)
	return rows, err
}

// named turns the lane's rows into meetings, each with who it is with.
func (m attentionMeetings) named(ctx context.Context, rows []crmcontracts.Activity) ([]attention.Meeting, error) {
	ahead := make([]attention.Meeting, 0, len(rows))
	for _, row := range rows {
		if !meetingStillWorthPreparing(row) || !meetingReadable(row) {
			continue
		}
		needsPrep, known := meetingPrep(row)
		ahead = append(ahead, attention.Meeting{
			ID: ids.UUID(row.Id), Subject: subjectOfMeeting(row), StartsAt: row.OccurredAt, Precision: row.OccurredPrecision,
			NeedsPrep: needsPrep, PrepKnown: known, HostUserID: hostOfMeeting(row),
		})
	}
	with, err := m.store.MeetingCounterparties(ctx, meetingIDs(ahead, func(meeting attention.Meeting) ids.UUID { return meeting.ID }))
	if err != nil {
		return nil, err
	}
	for i := range ahead {
		ahead[i].ContactID = with[ahead[i].ID]
	}
	return ahead, nil
}

// dayDatesOf is the store's day range for a lane window.
func dayDatesOf(window attention.MeetingWindow) *activities.DayDates {
	return &activities.DayDates{From: window.DaysFrom, Before: window.DaysUntil}
}

// meetingReadable re-checks a row the store was asked to return readable only.
// A withheld one here would be a gate that did not hold, and the cost is a
// subject on a screen it does not belong on, so it is checked, not assumed.
func meetingReadable(row crmcontracts.Activity) bool {
	return row.ContentState == nil || *row.ContentState == crmcontracts.ActivityContentStateAvailable
}

// applyMeetingScope turns the lane's scope into the store's meeting dials.
//
// One function, both lanes, because the two ask the same question of the same
// table from either side of a start time — and a mapping written twice is how
// one of them ends up answering "mine" with everybody's.
//
// It follows openTasksDueBy exactly, including where each answer comes FROM:
// "mine" is the acting reader, read off the context, while "owned by" is the
// named contact the caller passed. A false answer means there is no reader to
// answer for, which is a page of nothing rather than a refusal — reading every
// meeting and calling the result theirs is the widening this narrowing exists to
// prevent.
//
// TasksVisible sets no dial: the lane feed shows what the reader may see, which
// the row-scope gate and the audience arm have already decided.
func applyMeetingScope(
	ctx context.Context, in *activities.ListActivitiesInput, scope attention.TaskScope, owner ids.UUID,
) bool {
	switch scope {
	case attention.TasksMine:
		actor, ok := principal.Actor(ctx)
		if !ok || actor.UserID.IsZero() {
			return false
		}
		reader := ids.From[ids.UserKind](actor.UserID)
		in.OnMeetingOf = &reader
	case attention.TasksOwnedBy:
		if owner.IsZero() {
			return false
		}
		host := ids.From[ids.UserKind](owner)
		in.MeetingHost = &host
	case attention.TasksUnassigned:
		in.UnhostedMeetings = true
	case attention.TasksVisible:
	}
	return true
}

// hostOfMeeting is the seat whose calendar the meeting came off, zero when no
// calendar claims it — a meeting booked in the app, or captured before the host
// was recorded.
func hostOfMeeting(row crmcontracts.Activity) ids.UUID {
	if row.HostUserId == nil {
		return ids.UUID{}
	}
	return ids.UUID(*row.HostUserId)
}

// meetingStillWorthPreparing keeps the meetings a rep can still do something
// about: booked, rather than held, cancelled or a no-show. The time window is
// the database's to apply.
//
// A meeting with no status is treated as booked. Capture writes calendar events
// without one, and dropping them would empty this lane on exactly the
// installations whose calendars are connected.
func meetingStillWorthPreparing(row crmcontracts.Activity) bool {
	return row.MeetingStatus == nil || *row.MeetingStatus == crmcontracts.ActivityMeetingStatusBooked
}

// meetingPrep answers whether a meeting has anything written down for it, and
// whether that question could be answered at all.
//
// The signals are the two the row already carries: a body (the agenda or the
// notes somebody typed) and a link to a record outside this company (the
// customer the meeting is with). A meeting with neither is one nobody has
// prepared.
//
// It refuses to answer for a WITHHELD row, and that refusal is the point. A
// reader outside the activity's audience receives the row with its body nulled
// — content_state says so — so reading "no body" as "no agenda" would report
// every colleague's meeting as unprepared, to a reader who cannot open it to
// find out. Absent beats wrong: the caller draws nothing.
func meetingPrep(row crmcontracts.Activity) (needsPrep bool, known bool) {
	if !meetingReadable(row) {
		return false, false
	}
	if row.Body != nil && strings.TrimSpace(*row.Body) != "" {
		return false, true
	}
	if row.Links != nil && len(*row.Links) > 0 {
		return false, true
	}
	return true, true
}

// subjectOfMeeting is the line a meeting shows, or NOTHING.
//
// A calendar event with a blank title is a real thing a provider hands over,
// and the empty answer is the honest one: the product ships three languages, so
// a placeholder composed here reaches a German reader in English — and
// "(untitled meeting)" is a parenthetical stand-in rather than a sentence
// anybody wrote. The client writes "A meeting" in the reader's own words.
func subjectOfMeeting(row crmcontracts.Activity) string {
	if row.Subject != nil {
		return *row.Subject
	}
	return ""
}

// meetingIDs lists the meetings a lane kept, for the one read that says who
// each was with (activities.Store.MeetingCounterparties). Both lanes ask it, so
// one meeting names the same customer before and after it starts.
func meetingIDs[M any](meetings []M, id func(M) ids.UUID) []ids.UUID {
	out := make([]ids.UUID, 0, len(meetings))
	for _, meeting := range meetings {
		out = append(out, id(meeting))
	}
	return out
}

// attentionMeetingsAwaitingOutcome reads the meetings that already started and
// whose result nobody has recorded, through the same gated list.
//
// Oldest first in SQL, at the lane's limit. The meeting waiting longest has
// had a wrong record longest, and an ascending read keeps it on the page
// however deep the backlog is. A full page is the lane's cap, which
// boundedSources reports as more to come.
type attentionMeetingsAwaitingOutcome struct{ store *activities.Store }

func (m attentionMeetingsAwaitingOutcome) Since(
	ctx context.Context, window attention.MeetingWindow, limit int,
	scope attention.TaskScope, owner ids.UUID,
) ([]attention.MeetingAwaitingOutcome, error) {
	kind := string(crmcontracts.ActivityKindMeeting)
	in := activities.ListActivitiesInput{
		Kind: &kind, OccurredAfter: &window.From, OccurredBefore: &window.Until, DayDates: dayDatesOf(window),
		AwaitingOutcomeAsOf: &window.Until, Limit: &limit, Sort: &soonestFirst, ReadableOnly: true,
	}
	if !applyMeetingScope(ctx, &in, scope, owner) {
		return nil, nil
	}
	rows, _, err := m.store.ListActivities(ctx, in)
	if err != nil {
		return nil, err
	}
	over := make([]attention.MeetingAwaitingOutcome, 0, len(rows))
	for _, row := range rows {
		if !meetingReadable(row) {
			continue
		}
		over = append(over, attention.MeetingAwaitingOutcome{
			ID: ids.UUID(row.Id), Subject: subjectOfMeeting(row), StartedAt: row.OccurredAt, Precision: row.OccurredPrecision,
			Version: row.Version, HostUserID: hostOfMeeting(row), CalendarManaged: row.CalendarManaged != nil && *row.CalendarManaged,
			ReadOnly: row.Writable == nil || !*row.Writable,
		})
	}
	with, err := m.store.MeetingCounterparties(ctx, meetingIDs(over, func(meeting attention.MeetingAwaitingOutcome) ids.UUID { return meeting.ID }))
	if err != nil {
		return nil, err
	}
	for i := range over {
		over[i].ContactID = with[over[i].ID]
	}
	return over, nil
}
