// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The meetings lane's seam over the activities store. It answers two questions
// about a booked meeting: what to call it, and whether the reader is ready.

import (
	"context"

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
// that is already the lane. The day meetings and the timed ones are read with
// a limit each. A calendar's all-day rows lead the lane, and they can never
// take the places of the meetings about to start.
type attentionMeetings struct{ store *activities.Store }

// soonestFirst is the store's ascending order on the meeting's start.
var soonestFirst = "occurred_at"

func (m attentionMeetings) Today(
	ctx context.Context, window attention.MeetingWindow, limit int,
	scope attention.TaskScope, owner ids.UUID,
) ([]attention.Meeting, bool, error) {
	days := attention.MeetingWindow{From: window.From, Until: window.From, DaysFrom: window.DaysFrom, DaysUntil: window.DaysUntil}
	rows, err := m.openIn(ctx, days, limit+1, scope, owner)
	if err != nil {
		return nil, false, err
	}
	timed := attention.MeetingWindow{From: window.From, Until: window.Until, DaysFrom: window.DaysUntil, DaysUntil: window.DaysUntil}
	hours, err := m.openIn(ctx, timed, limit+1, scope, owner)
	if err != nil {
		return nil, false, err
	}
	// One row past the limit says whether a meeting was left out.
	full := len(rows) > limit || len(hours) > limit
	ahead, err := m.named(ctx, append(rows[:min(len(rows), limit)], hours[:min(len(hours), limit)]...))
	return ahead, full, err
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

// named turns the lane's rows into meetings, each with who it is with and the
// reader's own preparation for it.
func (m attentionMeetings) named(ctx context.Context, rows []crmcontracts.Activity) ([]attention.Meeting, error) {
	ahead := make([]attention.Meeting, 0, len(rows))
	for _, row := range rows {
		if !meetingStillWorthPreparing(row) || !meetingReadable(row) {
			continue
		}
		ahead = append(ahead, attention.Meeting{
			ID: ids.UUID(row.Id), Subject: subjectOfMeeting(row), StartsAt: row.OccurredAt, Precision: row.OccurredPrecision,
			HostUserID: hostOfMeeting(row),
		})
	}
	meetings := meetingIDs(ahead, func(meeting attention.Meeting) ids.UUID { return meeting.ID })
	with, err := m.store.MeetingCounterparties(ctx, meetings)
	if err != nil {
		return nil, err
	}
	prepared, err := m.store.MeetingPreparations(ctx, meetings)
	if err != nil {
		return nil, err
	}
	for i := range ahead {
		ahead[i].ContactID = with[ahead[i].ID]
		if own, attends := prepared[ahead[i].ID]; attends {
			wire := wirePreparation(own)
			ahead[i].Preparation = &wire
		}
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
