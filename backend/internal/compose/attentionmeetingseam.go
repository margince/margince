// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The meetings lane's seam over the activities store, and the two questions it
// answers about a booked meeting: what to call it, and whether anybody has
// prepared it.

import (
	"context"
	"sort"
	"strings"
	"time"

	"github.com/margince/margince/backend/internal/compose/attention"
	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/modules/activities"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/kernel/principal"
)

// attentionMeetings reads today's remaining meetings through the activities
// store — the same gated list every other activity surface reads.
//
// The WINDOW is applied in SQL, not here. An earlier cut read ten times the
// lane and narrowed the time range in Go, which is lossy in the one direction
// that hides itself: a day with more than the scan's worth of later activity
// pushes a real meeting off the page, and the lane draws a free afternoon over
// a booked one. ListActivitiesInput carries OccurredAfter/OccurredBefore and
// the store applies both as predicates, so the bound is the day rather than a
// guess about how busy the day might be.
//
// The STATUS filter stays in Go: the store has no dial for it, and the set it
// removes is bounded by the window the database already applied.
type attentionMeetings struct{ store *activities.Store }

func (m attentionMeetings) Today(
	ctx context.Context, from, until time.Time, limit int,
	scope attention.TaskScope, owner ids.UUID,
) ([]attention.Meeting, error) {
	kind := string(crmcontracts.ActivityKindMeeting)
	in := activities.ListActivitiesInput{
		Kind: &kind, OccurredAfter: &from, OccurredBefore: &until, Limit: &limit,
		ReadableOnly: true,
	}
	if !applyMeetingScope(ctx, &in, scope, owner) {
		return nil, nil
	}
	rows, _, err := m.store.ListActivities(ctx, in)
	if err != nil {
		return nil, err
	}
	ahead := make([]attention.Meeting, 0, len(rows))
	for _, row := range rows {
		if !meetingStillWorthPreparing(row) {
			continue
		}
		// The store was asked for readable rows only, so a withheld one here
		// would be a gate that did not hold. Checked rather than assumed: this
		// lane names the meeting in a reader's brief, and the cost of being
		// wrong is a subject on a screen it does not belong on.
		if row.ContentState != nil && *row.ContentState != crmcontracts.ActivityContentStateAvailable {
			continue
		}
		needsPrep, known := meetingPrep(row)
		ahead = append(ahead, attention.Meeting{
			ID: ids.UUID(row.Id), Subject: subjectOfMeeting(row), StartsAt: row.OccurredAt,
			NeedsPrep: needsPrep, PrepKnown: known, ContactID: contactOnMeeting(row),
			HostUserID: hostOfMeeting(row),
		})
	}
	// Soonest first: the lane is a countdown, and the store returns activities
	// newest-first, which is the opposite order for a day still ahead.
	sort.SliceStable(ahead, func(i, j int) bool { return ahead[i].StartsAt.Before(ahead[j].StartsAt) })
	return ahead, nil
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
	if row.ContentState != nil && *row.ContentState != crmcontracts.ActivityContentStateAvailable {
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

// contactOnMeeting is whose page this meeting's brief is read on.
//
// The FIRST contact link in the row's own order, which is the store's, so two
// reads of an unchanged meeting choose the same page. A meeting with several
// attendees has several honest answers and the row shows one link; picking by
// anything cleverer here would be a ranking this lane has no basis for, and
// picking a different one each read would move a control under the reader.
//
// Zero where the meeting links no contact at all — an internal meeting, or one
// whose attendees this reader may not see, since the links come back already
// scoped. The row then offers no brief rather than a link to somebody's page
// chosen at random.
func contactOnMeeting(row crmcontracts.Activity) ids.UUID {
	if row.Links == nil {
		return ids.UUID{}
	}
	for _, link := range *row.Links {
		if link.EntityType == crmcontracts.ActivityLinkEntityTypeContact {
			return ids.UUID(link.EntityId)
		}
	}
	return ids.UUID{}
}

// attentionMeetingsAwaitingOutcome reads the meetings that already started and
// whose result nobody has recorded, through the same gated list.
//
// The STATUS FILTER IS IN SQL here, unlike the forward lane above, and the
// difference is the direction. That lane's window is the rest of today, so the
// non-booked rows it drops in Go come out of a set the database already made
// small. This window reaches back a fortnight, where almost every meeting is
// settled: filtering after the read would spend the page on rows to discard and
// push the genuinely unreported meeting off the end, so the lane would draw
// "nothing to report" over real work. `AwaitingOutcome` asks the database
// instead.
//
// AND IT READS DEEPER THAN IT RENDERS, which the forward lane has no need to.
// The store orders `occurred_at DESC` (activityread.orderClause) and applies
// the limit in SQL, so asking it for the lane's cap returns the NEWEST rows in
// the window — while this lane shows the OLDEST first, because the meeting
// waiting longest is the one whose record has been wrong longest. Asked for
// exactly the cap, a fortnight holding more unanswered meetings than fit would
// hand back the freshest dozen and sort those, dropping the very rows the
// ordering exists to surface. So it reads a deeper page, sorts, and trims to
// the cap at the end.
type attentionMeetingsAwaitingOutcome struct{ store *activities.Store }

// unansweredReadDepth is how many rows the read above asks for per lane card.
//
// The multiple is what makes the oldest-first ordering true rather than
// approximately true: it is the number of unanswered meetings a fortnight may
// hold before the newest-first page stops containing the whole set. Deep enough
// that an ordinary backlog sorts correctly, bounded so a pathological one costs
// a page rather than a scan. A window holding more than this still renders its
// cap and still reports itself cut — the reader is told there is more, which is
// the honest answer — but the dozen it shows are no longer guaranteed to be the
// dozen oldest.
const unansweredReadDepth = 8

func (m attentionMeetingsAwaitingOutcome) Since(
	ctx context.Context, from, until time.Time, limit int,
	scope attention.TaskScope, owner ids.UUID,
) ([]attention.MeetingAwaitingOutcome, error) {
	kind := string(crmcontracts.ActivityKindMeeting)
	deep := limit * unansweredReadDepth
	in := activities.ListActivitiesInput{
		Kind: &kind, OccurredAfter: &from, OccurredBefore: &until,
		AwaitingOutcome: true, Limit: &deep, ReadableOnly: true,
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
		// Same defensive check the forward lane makes, for the same reason.
		if row.ContentState != nil && *row.ContentState != crmcontracts.ActivityContentStateAvailable {
			continue
		}
		over = append(over, attention.MeetingAwaitingOutcome{
			ID: ids.UUID(row.Id), Subject: subjectOfMeeting(row), StartedAt: row.OccurredAt,
			Version: row.Version, HostUserID: hostOfMeeting(row),
		})
	}
	// Longest unanswered first: the store returns activities newest-first, and
	// the meeting that ended last week has been waiting longer than the one that
	// ended ten minutes ago. A reader clearing the top of this lane is clearing
	// the oldest debt rather than the freshest.
	sort.SliceStable(over, func(i, j int) bool { return over[i].StartedAt.Before(over[j].StartedAt) })
	// Trimmed AFTER the sort, so what the cap keeps is the oldest rather than
	// whichever rows the database happened to return first. The caller bounds
	// the lane by what it renders, and the truncation flag counts this slice.
	if len(over) > limit {
		over = over[:limit]
	}
	return over, nil
}
