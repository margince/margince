// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
	"github.com/margince/margince/backend/internal/shared/ports/commsauthz"
)

func TestAnInstructionToMoveAnAppointmentNamesARequestedFollowup(t *testing.T) {
	t.Parallel()
	moves := []string{
		"move appointment", "Please reschedule our meeting to Thursday", "postpone the call by a week",
		"suggest a new time", "Termin verschieben", "Bitte den Termin auf Freitag verlegen", "Termin ändern",
		"einen neuen Termin vorschlagen", "dời lịch hẹn sang thứ Năm", "đổi giờ cuộc họp", "hoãn cuộc họp",
	}
	for _, intent := range moves {
		if got, _ := draftedReason(intent, ids.NewV7(), crmcontracts.ActivityKindEmail); got != commsauthz.CategoryRequestedFollowup {
			t.Errorf("%q → %q, want requested_followup", intent, got)
		}
	}
	others := []string{"", "send our new product brochure", "thank them for the meeting", "Angebot schicken", "gửi báo giá"}
	for _, intent := range others {
		if got, _ := draftedReason(intent, ids.NewV7(), crmcontracts.ActivityKindEmail); got != "" {
			t.Errorf("%q → %q, want no reason", intent, got)
		}
	}
}

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
