// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The two things a meeting that already happened must not be mistaken for.
//
// It shares the meetings CATEGORY with the lane it is the counterpart of —
// a reader filtering by "meetings" wants both — so the rules that key on
// category catch it by default, and both defaults are wrong for it.

import (
	"slices"
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
	"github.com/margince/margince/backend/internal/shared/kernel/ids"
)

// Nobody walks into a meeting that is over.
//
// preparesAConversation admits the whole meetings category, so without a rule
// of its own this row is filed under "prepare conversations" and tells a rep to
// get ready for something they already did. Recording what happened is repair.
func TestAMeetingThatHappenedIsNotSomethingToPrepareFor(t *testing.T) {
	t.Parallel()
	row := crmcontracts.WorklistItem{
		Source:   crmcontracts.WorklistItemSourceMeetingOutcome,
		Category: crmcontracts.WorklistItemCategoryMeetings,
	}
	if got := BriefSectionOf(row); got != crmcontracts.BriefSectionReviewAndRepair {
		t.Errorf("a meeting that happened is filed under %q, want %q — the row asks what "+
			"the meeting came to, and preparing for it is no longer possible",
			got, crmcontracts.BriefSectionReviewAndRepair)
	}
	// And the lane it is the counterpart of still prepares, so the rule above
	// separates the two rather than emptying the section.
	ahead := crmcontracts.WorklistItem{
		Source:   crmcontracts.WorklistItemSourceMeeting,
		Category: crmcontracts.WorklistItemCategoryMeetings,
	}
	if got := BriefSectionOf(ahead); got != crmcontracts.BriefSectionPrepareConversations {
		t.Errorf("a meeting still ahead is filed under %q, want %q", got,
			crmcontracts.BriefSectionPrepareConversations)
	}
}

// A full page of them says so.
//
// The lane reads at plannedCap, so a day holding that many unsettled meetings
// has more behind them. boundedSources is what turns that into a truncation
// flag; a source missing from it reports itself COMPLETE, which is the reading
// that hides work rather than the one that overstates it.
func TestAFullPageOfUnsettledMeetingsIsReportedAsCut(t *testing.T) {
	t.Parallel()
	full := make([]crmcontracts.AttentionItem, 0, plannedCap)
	for range plannedCap {
		full = append(full, item("m", "meeting_outcome"))
	}
	day := crmcontracts.Attention{MeetingsUnreported: &full}
	if !boundedSources(day)["meeting_outcome"] {
		t.Error("a lane filled to its bound reports itself complete, so a reader is told " +
			"there is nothing more to close off while there is")
	}
	short := full[:plannedCap-1]
	day.MeetingsUnreported = &short
	if boundedSources(day)["meeting_outcome"] {
		t.Error("a lane short of its bound reports itself cut, which would put a " +
			"there-may-be-more notice on a day that is genuinely clear")
	}
}

// A row that states an obligation and offers no way to meet it.
//
// This is what the sibling census cannot catch. TestNoLaneAdvertisesAVerbThe
// ClientCannotPerform asks the question in ONE direction — every verb sent has
// a client that performs it — and a source sending NO verb satisfies that
// vacuously. `meeting_outcome` shipped exactly that way: the row said a meeting
// owed an answer, and there was nothing to press.
//
// So the direction the census cannot see is asserted here, for this source. It
// is not a claim about every source: plenty of rows correctly offer no verb —
// a health card names something to fix elsewhere, and a meeting still ahead has
// no result yet. What makes silence wrong HERE is what the row says, which is
// that an answer is outstanding.
func TestAMeetingOwedAnAnswerOffersAWayToGiveIt(t *testing.T) {
	t.Parallel()
	item := meetingAwaitingOutcomeItem(MeetingAwaitingOutcome{
		ID:      ids.NewV7(),
		Subject: "Discovery call",
	})
	if len(item.Actions) == 0 {
		t.Fatal("a meeting that owes an answer offers no verb: the row tells a rep " +
			"something is unfinished and gives them no way to finish it, which is the " +
			"queue doing the opposite of its job")
	}
	if !slices.Contains(item.Actions, crmcontracts.AttentionItemActionsDecide) {
		t.Errorf("the row offers %v, want decide — the answer is given ON the row, the "+
			"way an approval's is", item.Actions)
	}
}

// The verb writes the activity, so the row has to name the version it is
// answered against — the client cannot make the write conditional otherwise,
// and two readers answering one meeting both succeed with the later one
// winning silently.
//
// Asserted on the ITEM rather than trusted from the lane type: a version that
// never reaches the wire is a version the browser cannot send, and the
// frontend's If-Match census can only see that the call spells `ifMatch`, not
// that the number arrives.
func TestAMeetingOwedAnAnswerNamesTheVersionItIsAnsweredAgainst(t *testing.T) {
	t.Parallel()
	version := int64(7)
	item := meetingAwaitingOutcomeItem(MeetingAwaitingOutcome{
		ID: ids.NewV7(), Subject: "Discovery call", Version: &version,
	})
	if item.Version == nil {
		t.Fatal("the row carries no version: the answer cannot be made conditional, so two " +
			"readers deciding one meeting overwrite each other and the second is told nothing")
	}
	if *item.Version != version {
		t.Errorf("version = %d, want %d", *item.Version, version)
	}
}
