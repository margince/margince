// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The way into a meeting's brief.
//
// The row could describe a meeting and offer no way to prepare for it, which is
// the one thing a rep opens that row to do. The brief is not a page of its own —
// it opens as `?prep=<activity>` on a PERSON's record — so naming it needs two
// ids and the row carried only one.
//
// Both halves are tested here because a move that names one id is not half a
// control, it is a control that opens nothing.

import (
	"testing"
	"time"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

func TestAMeetingWithSomebodyOnItReachesTheBrief(t *testing.T) {
	person := ids.NewV7()
	meeting := ids.NewV7()
	row := classifyMeeting(meetingItem(Meeting{
		ID: meeting, Subject: "Fleet retrofit review",
		StartsAt: fixedClock().Add(2 * time.Hour), PersonID: person,
	}), fixedClock())

	if row.item.Move == nil {
		t.Fatal("the row offers no move, so a rep reading it cannot prepare")
	}
	if row.item.Move.Action != crmcontracts.WorklistMoveActionOpenMeetingBrief {
		t.Errorf("the move is %q, want open_meeting_brief", row.item.Move.Action)
	}
	// WHICH meeting. Without it the brief has nothing to prepare.
	if row.item.Move.ActivityId == nil || ids.UUID(*row.item.Move.ActivityId) != meeting {
		t.Errorf("the move names meeting %v, want %v", row.item.Move.ActivityId, meeting)
	}
	// WHOSE page. Without it the address has no record to open on, and the
	// client draws nothing — which is the failure this whole change is about.
	if row.item.WithPerson == nil || ids.UUID(*row.item.WithPerson) != person {
		t.Errorf("the row names person %v, want %v", row.item.WithPerson, person)
	}
}

// A meeting naming nobody this reader may see offers NO way in.
//
// An internal meeting has no attendee, and so does one whose only attendees are
// people the caller cannot read — the lane cannot tell those apart and does not
// need to, because both mean the same thing: there is no page to read this
// brief on. A move sent anyway would be a control that opens nothing, which is
// worse than a row that admits it has no step.
func TestAMeetingNamingNobodyOffersNoBrief(t *testing.T) {
	row := classifyMeeting(meetingItem(Meeting{
		ID: ids.NewV7(), Subject: "Internal planning",
		StartsAt: fixedClock().Add(2 * time.Hour),
	}), fixedClock())

	if row.item.Move != nil {
		t.Errorf("the row offers %q into a brief it has no page for", row.item.Move.Action)
	}
	if row.item.WithPerson != nil {
		t.Errorf("the row names person %v on a meeting that named nobody", row.item.WithPerson)
	}
}
