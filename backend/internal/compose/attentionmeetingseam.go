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
) ([]attention.Meeting, error) {
	kind := string(crmcontracts.ActivityKindMeeting)
	rows, _, err := m.store.ListActivities(ctx, activities.ListActivitiesInput{
		Kind: &kind, OccurredAfter: &from, OccurredBefore: &until, Limit: &limit,
	})
	if err != nil {
		return nil, err
	}
	ahead := make([]attention.Meeting, 0, len(rows))
	for _, row := range rows {
		if !meetingStillWorthPreparing(row) {
			continue
		}
		needsPrep, known := meetingPrep(row)
		ahead = append(ahead, attention.Meeting{
			ID: ids.UUID(row.Id), Subject: subjectOfMeeting(row), StartsAt: row.OccurredAt,
			NeedsPrep: needsPrep, PrepKnown: known, PersonID: personOnMeeting(row),
		})
	}
	// Soonest first: the lane is a countdown, and the store returns activities
	// newest-first, which is the opposite order for a day still ahead.
	sort.SliceStable(ahead, func(i, j int) bool { return ahead[i].StartsAt.Before(ahead[j].StartsAt) })
	return ahead, nil
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
// notes somebody typed) and a link to a record outside this organization (the
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

// personOnMeeting is whose page this meeting's brief is read on.
//
// The FIRST person link in the row's own order, which is the store's, so two
// reads of an unchanged meeting choose the same page. A meeting with several
// attendees has several honest answers and the row shows one link; picking by
// anything cleverer here would be a ranking this lane has no basis for, and
// picking a different one each read would move a control under the reader.
//
// Zero where the meeting links no person at all — an internal meeting, or one
// whose attendees this reader may not see, since the links come back already
// scoped. The row then offers no brief rather than a link to somebody's page
// chosen at random.
func personOnMeeting(row crmcontracts.Activity) ids.UUID {
	if row.Links == nil {
		return ids.UUID{}
	}
	for _, link := range *row.Links {
		if link.EntityType == crmcontracts.ActivityLinkEntityTypePerson {
			return ids.UUID(link.EntityId)
		}
	}
	return ids.UUID{}
}
