// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package activities

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
)

// The planner uses activity_identity_meeting_uid only for the expression it was
// built on, so a reworded reader would scan every meeting identity instead.
func TestTheMeetingUIDIndexIsBuiltOnTheReadersOwnExpression(t *testing.T) {
	t.Parallel()
	found, err := filepath.Glob("../../../migrations/core/*_a_meeting_is_found_by_its_event_uid.up.sql")
	if err != nil || len(found) != 1 {
		t.Fatalf("the index migration: %v, %v", found, err)
	}
	ddl, err := os.ReadFile(found[0])
	if err != nil {
		t.Fatal(err)
	}
	if want := meetingUIDOfKeySQL("identity_key"); !strings.Contains(string(ddl), want) {
		t.Errorf("the index does not build on %q", want)
	}
}

// An invitation a reader may answer asks for an RSVP, so the row stops claiming
// it is owed a written reply even when the classifier said it asks us.
func TestAnOpenInvitationOwesNoWrittenReply(t *testing.T) {
	t.Parallel()
	summary := crmcontracts.EmailSummary{Move: crmcontracts.EmailSummaryMoveNeedsReply}
	openInvitation{response: crmcontracts.EmailCalendarInvitationResponseNeedsAction}.mark(&summary)
	if summary.Move != crmcontracts.EmailSummaryMoveNone {
		t.Errorf("an open invitation reads move %q, want none", summary.Move)
	}
	if summary.CalendarInvitation == nil || summary.CalendarInvitation.Response != crmcontracts.EmailCalendarInvitationResponseNeedsAction {
		t.Errorf("the row carries %+v, want the reader's open answer", summary.CalendarInvitation)
	}
}
