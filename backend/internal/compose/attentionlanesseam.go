// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package compose

// The three OPTIONAL attention lanes, bound to the engines that already own
// what they read.
//
// Each is a binding rather than an implementation, which is the point: the
// promises come from the people module's claim read, the deal risk from the
// same candidate engine whats_slipping_this_week reads, and the meetings from
// the activities list every other activity surface reads. A lane that derived
// its own answer here would be a second opinion the product would have to keep
// agreeing with.

import (
	"context"
	"sort"
	"time"

	"github.com/gradionhq/margince/backend/internal/compose/attention"
	crmcontracts "github.com/gradionhq/margince/backend/internal/contracts"
	"github.com/gradionhq/margince/backend/internal/modules/activities"
	"github.com/gradionhq/margince/backend/internal/modules/agents"
	"github.com/gradionhq/margince/backend/internal/modules/people"
	"github.com/gradionhq/margince/backend/internal/shared/apperrors"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/ids"
	"github.com/gradionhq/margince/backend/internal/shared/kernel/principal"
)

// attentionCommitments reads the acting rep's own promises through the people
// store.
//
// A claim carries no assignee, so ownership rides the person it was made to:
// the rep who holds the relationship is the one who made the promise in their
// own captured conversation. A principal with no human behind it has no
// promises of its own to keep, which is a refusal rather than an empty lane —
// the feed omits and NAMES the lane instead of reporting a clear day.
type attentionCommitments struct{ store *people.Store }

func (c attentionCommitments) DueBy(ctx context.Context, by time.Time, limit int) ([]attention.Commitment, error) {
	actor, ok := principal.Actor(ctx)
	if !ok || actor.UserID.IsZero() {
		return nil, apperrors.ErrPermissionDenied
	}
	due, err := c.store.OpenCommitmentsDue(ctx, ids.From[ids.UserKind](actor.UserID), by, limit)
	if err != nil {
		return nil, err
	}
	promises := make([]attention.Commitment, 0, len(due))
	for _, row := range due {
		promises = append(promises, attention.Commitment{
			ID:          row.ID,
			PersonID:    row.PersonID.UUID,
			Body:        row.Body,
			Quote:       row.SourceQuote,
			SourceLabel: row.SourceLabel,
			OccurredAt:  row.OccurredAt,
			DueAt:       row.DueAt,
		})
	}
	return promises, nil
}

// attentionAtRisk reads the pipeline's own risk candidates at the morning
// queue's shorter idle window.
//
// It calls quietDealLister, the SAME engine whats_slipping_this_week reads, so
// there is one at-risk rule in the product and the two surfaces cannot come to
// disagree about which deals are in trouble. Only the patience differs, and it
// is named here rather than buried: a queue exists to warn, and the stalled
// threshold is a status rather than a warning.
type attentionAtRisk struct{ lister agents.SlippingLister }

func (a attentionAtRisk) Quiet(ctx context.Context) ([]attention.RiskyDeal, error) {
	candidates, err := a.lister(ctx)
	if err != nil {
		return nil, err
	}
	now := clockNow()
	risky := make([]attention.RiskyDeal, 0, len(candidates))
	for _, deal := range candidates {
		risky = append(risky, attention.RiskyDeal{
			DealID:            deal.DealID,
			Name:              deal.Name,
			QuietDays:         idleDaysOf(deal, now),
			CloseOverdue:      deal.CloseOverdue,
			ExpectedCloseDate: deal.ExpectedCloseDate,
		})
	}
	return risky, nil
}

// idleDaysOf is how long the deal has been quiet, counted from the same base
// the idle rule itself measures from: the last activity, or the deal's creation
// when nothing has ever touched it.
func idleDaysOf(deal agents.SlippingDeal, now time.Time) int {
	since := deal.CreatedAt
	if deal.LastActivityAt != nil {
		since = *deal.LastActivityAt
	}
	days := int(now.Sub(since).Hours() / 24)
	if days < 0 {
		return 0
	}
	return days
}

// attentionMeetings reads today's remaining meetings through the activities
// store — the same gated list every other activity surface reads.
//
// SCAN AND FILTER, like the task lane above and for the same reason: the store
// cannot express "kind=meeting between two instants", so this reads wider and
// narrows here. The same scan factor applies, because a day with a pile of
// finished meetings would otherwise push the one still ahead off the page.
type attentionMeetings struct{ store *activities.Store }

func (m attentionMeetings) Today(
	ctx context.Context, from, until time.Time, limit int,
) ([]attention.Meeting, error) {
	kind := string(crmcontracts.ActivityKindMeeting)
	scan := limit * taskScanFactor
	rows, _, err := m.store.ListActivities(ctx, activities.ListActivitiesInput{Kind: &kind, Limit: &scan})
	if err != nil {
		return nil, err
	}
	ahead := make([]attention.Meeting, 0, len(rows))
	for _, row := range rows {
		if !meetingStillWorthPreparing(row, from, until) {
			continue
		}
		ahead = append(ahead, attention.Meeting{
			ID: ids.UUID(row.Id), Subject: subjectOfMeeting(row), StartsAt: row.OccurredAt,
		})
	}
	// Soonest first: the lane is a countdown, and the store returns activities
	// newest-first, which is the opposite order for a day still ahead.
	sort.SliceStable(ahead, func(i, j int) bool { return ahead[i].StartsAt.Before(ahead[j].StartsAt) })
	if len(ahead) > limit {
		ahead = ahead[:limit]
	}
	return ahead, nil
}

// meetingStillWorthPreparing keeps the meetings a rep can still do something
// about: booked (not held, not cancelled, not a no-show) and starting between
// now and the end of the day.
//
// A meeting with no status is treated as booked. Capture writes calendar events
// without one, and dropping them would empty this lane on exactly the
// installations whose calendars are connected.
func meetingStillWorthPreparing(row crmcontracts.Activity, from, until time.Time) bool {
	if row.MeetingStatus != nil && *row.MeetingStatus != crmcontracts.ActivityMeetingStatusBooked {
		return false
	}
	return !row.OccurredAt.Before(from) && row.OccurredAt.Before(until)
}

// subjectOfMeeting is the line a meeting shows. Unlike a task, a meeting may
// honestly have no subject — a calendar event with a blank title is a real
// thing a provider hands over — so the fallback is a routine case here.
func subjectOfMeeting(row crmcontracts.Activity) string {
	if row.Subject != nil && *row.Subject != "" {
		return *row.Subject
	}
	return "(untitled meeting)"
}
