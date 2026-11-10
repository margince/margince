// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package projects

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestTheHealthJobWritesOnlyWhatNobodyHasSaidSinceTheLastActivity(t *testing.T) {
	t.Parallel()
	activity := time.Date(2026, 10, 1, 9, 0, 0, 0, time.UTC)
	before, after := activity.Add(-time.Hour), activity.Add(time.Hour)
	reading := func(state crmcontracts.ProjectHealthState, source string) *crmcontracts.ProjectHealthAssessment {
		return &crmcontracts.ProjectHealthAssessment{State: state, Source: &source}
	}
	const (
		onTrack  = crmcontracts.ProjectHealthStateOnTrack
		atRisk   = crmcontracts.ProjectHealthStateAtRisk
		offTrack = crmcontracts.ProjectHealthStateOffTrack
	)
	for _, tc := range []struct {
		name       string
		current    *crmcontracts.ProjectHealthAssessment
		userSaidAt *time.Time
		proposed   crmcontracts.ProjectHealthState
		want       AIHealthOutcome
	}{
		{"nobody judged it and it is in trouble", nil, nil, atRisk, AIHealthWritten},
		{"nobody judged it and nothing is wrong", nil, nil, onTrack, AIHealthUnchanged},
		{"a user spoke after the last activity", reading(onTrack, sourceHuman), &after, atRisk, AIHealthHeldByUser},
		{"a user spoke at the instant of the last activity", reading(onTrack, sourceHuman), &activity, atRisk, AIHealthHeldByUser},
		{"a user spoke before the last activity", reading(onTrack, sourceHuman), &before, atRisk, AIHealthWritten},
		{"an old user reading is not overruled with on_track", reading(atRisk, sourceHuman), &before, onTrack, AIHealthUnchanged},
		{"an old user reading already says it", reading(atRisk, sourceHuman), &before, atRisk, AIHealthUnchanged},
		{"an undo after the last activity holds the job off", nil, &after, offTrack, AIHealthHeldByUser},
		{"the job's own reading says the same", reading(atRisk, sourceAI), nil, atRisk, AIHealthUnchanged},
		{"the job's own reading worsens", reading(atRisk, sourceAI), nil, offTrack, AIHealthWritten},
		{"the job takes back its own alarm", reading(atRisk, sourceAI), &before, onTrack, AIHealthWritten},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			standing := aiHealthStanding{Current: tc.current, UserSaidAt: tc.userSaidAt, ActiveAt: activity}
			if got := standing.decide(tc.proposed); got != tc.want {
				t.Fatalf("decide(%s) = %d, want %d", tc.proposed, got, tc.want)
			}
		})
	}
}
