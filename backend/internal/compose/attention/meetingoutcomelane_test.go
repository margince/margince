// SPDX-License-Identifier: BUSL-1.1
// SPDX-FileCopyrightText: 2026 Gradion

package attention

// The two things a meeting that already happened must not be mistaken for.
//
// It shares the meetings CATEGORY with the lane it is the counterpart of —
// a reader filtering by "meetings" wants both — so the rules that key on
// category catch it by default, and both defaults are wrong for it.

import (
	"testing"

	crmcontracts "github.com/margince/margince/backend/internal/contracts"
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
