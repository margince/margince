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
)

// HealthReason is one factor's number and the fact behind it.
type HealthReason struct {
	Key    string
	Value  float64
	Reason string
}

// HealthReasons renders the four factors in the order they weigh.
func HealthReasons(h DealHealth, now time.Time) []HealthReason {
	ev := h.Evidence
	return []HealthReason{
		{Key: "activity_recency", Value: h.Factors.ActivityRecency, Reason: recencyReason(ev.LastActivityAt, ev.Stalled, now)},
		{
			Key: "stage_velocity", Value: h.Factors.StageVelocity,
			Reason: fmt.Sprintf("%d days in this stage; won deals spent about %d.", int(ev.DaysInStage), int(ev.ExpectedDaysInStage)),
		},
		{Key: "engagement", Value: h.Factors.Engagement, Reason: engagementReason(len(ev.EngagedStakeholderIDs))},
		{Key: "commitments", Value: h.Factors.Commitments, Reason: commitmentsReason(len(ev.OverdueTaskIDs))},
	}
}

func recencyReason(last *time.Time, stalled bool, now time.Time) string {
	if last == nil {
		return "No activity has been logged on this deal."
	}
	days := int(now.Sub(*last).Hours() / 24)
	when := fmt.Sprintf("%d days ago", days)
	switch days {
	case 0:
		when = "today"
	case 1:
		when = "yesterday"
	}
	if stalled {
		return fmt.Sprintf("Last activity %s — the deal counts as stalled.", when)
	}
	return fmt.Sprintf("Last activity %s.", when)
}

func engagementReason(engaged int) string {
	switch engaged {
	case 0:
		return "No stakeholder has been in two-way contact in the last 90 days."
	case 1:
		return "One stakeholder in two-way contact in the last 90 days."
	default:
		return fmt.Sprintf("%d stakeholders in two-way contact in the last 90 days.", engaged)
	}
}

func commitmentsReason(overdue int) string {
	switch overdue {
	case 0:
		return "No overdue task on this deal."
	case 1:
		return "One task is overdue."
	default:
		return fmt.Sprintf("%d tasks are overdue.", overdue)
	}
}
