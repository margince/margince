// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package deals

// The words behind the health numbers.
//
// A factor of 0.31 is nobody's to disagree with; "Last activity 24 days ago —
// the deal counts as stalled" is. These render the fact each factor was read
// from, so a reader can argue with the reading rather than only with the mood
// it puts them in.
//
// They live in this module because the formula does. The deal status card is
// their caller — it hands the sentences to the writer that composes the card —
// and a second spelling of "no overdue task" beside a different threshold is
// how the card and the number start disagreeing in front of a user.

import (
	"fmt"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/elapsed"
	"github.com/margince/margince/backend/internal/shared/kernel/langcopy"
)

// HealthReason is one factor's number and the fact behind it.
type HealthReason struct {
	Key    string
	Value  float64
	Reason string
}

// HealthReasons renders the four factors in the order they weigh.
func HealthReasons(h DealHealth, now time.Time, loc *time.Location, language string) []HealthReason {
	ev := h.Evidence
	said := langcopy.For(language)
	return []HealthReason{
		{Key: "activity_recency", Value: h.Factors.ActivityRecency, Reason: recencyReason(ev.LastActivityAt, ev.Stalled, now, loc, said)},
		{
			Key: "stage_velocity", Value: h.Factors.StageVelocity,
			Reason: fmt.Sprintf(said.Say(healthWords.Stage), int(ev.DaysInStage), int(ev.ExpectedDaysInStage)),
		},
		{Key: "engagement", Value: h.Factors.Engagement, Reason: engagementReason(len(ev.EngagedStakeholderIDs), said)},
		{Key: "commitments", Value: h.Factors.Commitments, Reason: commitmentsReason(len(ev.OverdueTaskIDs), said)},
	}
}

// recencyReason says how long ago the last activity was, BY THE CALENDAR.
//
// Not by elapsed hours, and the difference is one a reader can catch the
// product in. The deal status card is cached on a fingerprint that includes the
// day in the installation zone, so a sentence whose words change on the elapsed clock changes while
// the key stands still: last activity at 14:00 on the 20th, a card written at
// 09:00 on the 22nd says "1 days ago", and at 14:00 that afternoon the sentence
// is wrong and stays wrong until midnight because nothing has invalidated it.
//
// Counting the same way the key does means the wording can only change when the
// key does. It is also how a reader looking at two dates counts: 23:00 Monday
// to 01:00 Tuesday is one day, not zero.
//
// The SCORE is a separate question and keeps its fractional elapsed count
// (health.daysBetween). A number that measures how stale a deal is should move
// continuously; a sentence a reader can check against a timestamp should not
// move between cache writes.
func recencyReason(last *time.Time, stalled bool, now time.Time, loc *time.Location, said langcopy.Spoken) string {
	if last == nil {
		return said.Say(healthWords.NoActivity)
	}
	days := elapsed.Days(*last, now, loc)
	when := fmt.Sprintf(said.Say(healthWords.DaysAgo), days)
	switch {
	case days <= 0:
		when = said.Say(healthWords.Today)
	case days == 1:
		when = said.Say(healthWords.Yesterday)
	}
	if stalled {
		return fmt.Sprintf(said.Say(healthWords.Stalled), when)
	}
	return fmt.Sprintf(said.Say(healthWords.Active), when)
}

func engagementReason(engaged int, said langcopy.Spoken) string {
	switch engaged {
	case 0:
		return fmt.Sprintf(said.Say(healthWords.NoEngagement), healthEngagementWindowDays)
	case 1:
		return fmt.Sprintf(said.Say(healthWords.OneEngagement), healthEngagementWindowDays)
	default:
		return fmt.Sprintf(said.Say(healthWords.ManyEngaged), engaged, healthEngagementWindowDays)
	}
}

func commitmentsReason(overdue int, said langcopy.Spoken) string {
	switch overdue {
	case 0:
		return said.Say(healthWords.NoOverdue)
	case 1:
		return said.Say(healthWords.OneOverdue)
	default:
		return fmt.Sprintf(said.Say(healthWords.ManyOverdue), overdue)
	}
}
