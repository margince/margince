// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// The meeting is evidence only when the message is written from it.
func TestAMovedMeetingIsTheEvidenceOnlyWhenItIsTheAnchor(t *testing.T) {
	t.Parallel()
	meeting := ids.NewV7()
	if _, evidence := draftedReason("move appointment", meeting, crmcontracts.ActivityKindMeeting); evidence.ActivityID != meeting {
		t.Errorf("evidence = %v, want the meeting %v", evidence.ActivityID, meeting)
	}
	if _, evidence := draftedReason("move appointment", meeting, crmcontracts.ActivityKindEmail); !evidence.ActivityID.IsZero() {
		t.Errorf("an email anchor named %v as the meeting", evidence.ActivityID)
	}
}
