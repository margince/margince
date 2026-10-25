// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"
	"time"

	"github.com/margince/margince/backend/internal/shared/kernel/occurredday"
)

// In Auckland a day source's stored noon is the next local morning. A day note
// dated today has been said. A day meeting dated today is pending until its date ends.
func TestADaySourceIsAdmittedByItsDate(t *testing.T) {
	t.Parallel()
	auckland, err := time.LoadLocation("Pacific/Auckland")
	if err != nil {
		t.Fatal(err)
	}
	morning := time.Date(2026, 9, 2, 8, 0, 0, 0, auckland)
	today := occurredday.AnchorIn(morning, auckland)
	for name, c := range map[string]struct {
		meeting bool
		want    CommitmentAdmissionReason
	}{
		"a day note dated today":    {false, CommitmentCurrent},
		"a day meeting dated today": {true, CommitmentMeetingPending},
	} {
		evidence := CommitmentAdmissionEvidence{OccurredAt: today, DateOnly: true, Meeting: c.meeting, ContextAssessed: true}
		if got := AdmitAutomaticCommitment(evidence, morning, auckland, nil); got != c.want {
			t.Errorf("%s: admitted as %q, want %q", name, got, c.want)
		}
	}
}
