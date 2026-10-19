// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

func TestOnlyChangedFollowupEvidenceHoldsAScheduledMessage(t *testing.T) {
	created := time.Date(2026, 11, 2, 9, 0, 0, 0, time.UTC)
	done := true
	canceled := crmcontracts.ActivityMeetingStatusCanceled
	for _, scenario := range []struct {
		source crmcontracts.Activity
		want   string
	}{
		{crmcontracts.Activity{Kind: crmcontracts.ActivityKindTask, UpdatedAt: created.Add(-time.Hour), IsDone: &done}, ""},
		{crmcontracts.Activity{Kind: crmcontracts.ActivityKindTask, UpdatedAt: created.Add(time.Hour), IsDone: &done}, HeldFollowupSettled},
		{crmcontracts.Activity{Kind: crmcontracts.ActivityKindMeeting, UpdatedAt: created.Add(time.Hour), MeetingStatus: &canceled}, HeldMeetingCanceled},
		{crmcontracts.Activity{Kind: crmcontracts.ActivityKindTask, UpdatedAt: created.Add(time.Hour)}, ""},
		{crmcontracts.Activity{Kind: crmcontracts.ActivityKindMeeting, UpdatedAt: created.Add(time.Hour), IsDone: &done}, ""},
	} {
		if got := changedFollowup(scenario.source, created); got != scenario.want {
			t.Errorf("context hold = %s, want %s", got, scenario.want)
		}
	}
}
